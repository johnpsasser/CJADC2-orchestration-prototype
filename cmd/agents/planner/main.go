// Planner Agent - Generates action proposals based on correlated tracks and threat levels
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
	"github.com/agile-defense/cjadc2/pkg/opa"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// PlannerAgent generates action proposals for correlated tracks
type PlannerAgent struct {
	*agent.BaseAgent
	logger           zerolog.Logger
	consumer         jetstream.Consumer
	opaClient        *opa.Client
	proposalsCreated prometheus.Counter
	proposalsDenied  prometheus.Counter
}

// NewPlannerAgent creates a new planner agent
func NewPlannerAgent(cfg agent.Config) (*PlannerAgent, error) {
	base, err := agent.NewBaseAgent(cfg)
	if err != nil {
		return nil, err
	}

	// Additional metrics
	proposalsCreated := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "planner_proposals_created_total",
		Help: "Total number of proposals created",
	})

	proposalsDenied := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "planner_proposals_denied_total",
		Help: "Total number of proposals denied by policy",
	})

	base.Metrics().MustRegister(proposalsCreated, proposalsDenied)

	return &PlannerAgent{
		BaseAgent:        base,
		logger:           *base.Logger(),
		opaClient:        opa.NewClient(cfg.OPAUrl),
		proposalsCreated: proposalsCreated,
		proposalsDenied:  proposalsDenied,
	}, nil
}

// Run starts the planner agent
func (a *PlannerAgent) Run(ctx context.Context) error {
	// Start base agent (connects to NATS)
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base agent: %w", err)
	}

	// Ensure streams exist
	if err := natsutil.SetupStreams(ctx, a.JetStream()); err != nil {
		return fmt.Errorf("failed to setup streams: %w", err)
	}

	// Create consumer for correlated tracks
	consumer, err := natsutil.SetupConsumer(ctx, a.JetStream(), "TRACKS", "planner")
	if err != nil {
		return fmt.Errorf("failed to setup consumer: %w", err)
	}
	a.consumer = consumer

	a.logger.Info().Msg("Planner agent started, consuming from TRACKS stream")

	// Start consuming messages
	return a.consumeMessages(ctx)
}

// consumeMessages processes correlated track messages
func (a *PlannerAgent) consumeMessages(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch messages with timeout
		msgs, err := a.consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if err == context.DeadlineExceeded || err == context.Canceled {
				continue
			}
			a.logger.Error().Err(err).Msg("Failed to fetch messages")
			a.RecordError("fetch_error")
			time.Sleep(time.Second)
			continue
		}

		for msg := range msgs.Messages() {
			if err := a.processMessage(ctx, msg); err != nil {
				a.logger.Error().Err(err).Msg("Failed to process message")
				a.RecordError("process_error")
				msg.Nak()
			} else {
				msg.Ack()
			}
		}

		if msgs.Error() != nil && msgs.Error() != context.DeadlineExceeded {
			a.logger.Warn().Err(msgs.Error()).Msg("Message batch error")
		}
	}
}

// processMessage handles a single correlated track message
func (a *PlannerAgent) processMessage(ctx context.Context, msg jetstream.Msg) error {
	start := time.Now()

	// Parse correlated track
	var track messages.CorrelatedTrack
	if err := json.Unmarshal(msg.Data(), &track); err != nil {
		return fmt.Errorf("failed to unmarshal correlated track: %w", err)
	}

	correlationID := track.Envelope.CorrelationID
	if correlationID == "" {
		correlationID = track.Envelope.MessageID
	}

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("track_id", track.TrackID).
		Str("threat_level", track.ThreatLevel).
		Str("classification", track.Classification).
		Msg("Processing correlated track")

	// Generate action proposal
	proposal := a.generateProposal(&track)

	// Validate proposal with OPA
	decision, err := a.validateProposal(ctx, proposal, &track)
	if err != nil {
		a.logger.Warn().
			Err(err).
			Str("correlation_id", correlationID).
			Msg("OPA validation failed, proceeding with warning")
		// Add warning to proposal but still proceed
		proposal.PolicyDecision = messages.PolicyDecision{
			Allowed:  true,
			Warnings: []string{fmt.Sprintf("OPA validation error: %v", err)},
		}
	} else {
		proposal.PolicyDecision = messages.PolicyDecision{
			Allowed:    decision.Allowed,
			Reasons:    decision.Reasons,
			Violations: decision.Violations,
			Warnings:   decision.Warnings,
		}

		if !decision.Allowed {
			a.proposalsDenied.Inc()
			a.logger.Warn().
				Str("correlation_id", correlationID).
				Strs("reasons", decision.Reasons).
				Msg("Proposal denied by policy")
			// Still publish for audit, but mark as policy-denied
		}
	}

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("proposal_id", proposal.ProposalID).
		Str("action_type", proposal.ActionType).
		Int("priority", proposal.Priority).
		Bool("policy_allowed", proposal.PolicyDecision.Allowed).
		Msg("Proposal generated")

	// Publish to PROPOSALS stream
	subject := proposal.Subject()
	data, err := json.Marshal(proposal)
	if err != nil {
		return fmt.Errorf("failed to marshal proposal: %w", err)
	}

	_, err = a.JetStream().Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish proposal: %w", err)
	}

	duration := time.Since(start)
	a.RecordMessage("success", "correlated_track")
	a.RecordLatency("correlated_track", duration)
	a.proposalsCreated.Inc()

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("subject", subject).
		Dur("latency_ms", duration).
		Msg("Published action proposal")

	return nil
}

// generateProposal creates an action proposal based on the track
func (a *PlannerAgent) generateProposal(track *messages.CorrelatedTrack) *messages.ActionProposal {
	proposal := messages.NewActionProposal(track, a.ID())
	proposal.ProposalID = uuid.New().String()

	// Determine action type and priority based on threat level and classification
	actionType, priority, rationale := a.determineAction(track)
	proposal.ActionType = actionType
	proposal.Priority = priority
	proposal.Rationale = rationale

	// Set constraints based on the action
	proposal.Constraints = a.determineConstraints(track, actionType)

	// Set expiration based on priority
	expiration := a.determineExpiration(priority)
	proposal.ExpiresAt = time.Now().UTC().Add(expiration)

	return proposal
}

// determineAction decides what action to take based on track characteristics
func (a *PlannerAgent) determineAction(track *messages.CorrelatedTrack) (actionType string, priority int, rationale string) {
	classification := track.Classification
	threatLevel := track.ThreatLevel
	trackType := track.Type

	// Critical threat - immediate engagement consideration
	if threatLevel == "critical" {
		if classification == "hostile" && trackType == "missile" {
			return "engage", 10, fmt.Sprintf(
				"Critical threat: hostile missile detected at position (%.4f, %.4f) with speed %.1f m/s. Immediate defensive action recommended.",
				track.Position.Lat, track.Position.Lon, track.Velocity.Speed,
			)
		}
		return "intercept", 9, fmt.Sprintf(
			"Critical threat: %s %s requires immediate interception.",
			classification, trackType,
		)
	}

	// High threat - intercept or identify
	if threatLevel == "high" {
		if classification == "hostile" {
			return "intercept", 8, fmt.Sprintf(
				"High threat: hostile %s approaching. Interception recommended for defensive posture.",
				trackType,
			)
		}
		if classification == "unknown" {
			return "identify", 7, fmt.Sprintf(
				"High threat unknown %s detected. Identification required before further action.",
				trackType,
			)
		}
	}

	// Medium threat - track or identify
	if threatLevel == "medium" {
		if classification == "unknown" {
			return "identify", 5, fmt.Sprintf(
				"Medium threat: unknown %s requires identification.",
				trackType,
			)
		}
		if classification == "hostile" {
			return "track", 6, fmt.Sprintf(
				"Medium threat: hostile %s should be tracked for situational awareness.",
				trackType,
			)
		}
	}

	// Low threat - monitor or ignore
	if threatLevel == "low" {
		if classification == "friendly" {
			return "monitor", 2, fmt.Sprintf(
				"Friendly %s detected. Continued monitoring for coordination.",
				trackType,
			)
		}
		if classification == "neutral" {
			return "monitor", 3, fmt.Sprintf(
				"Neutral %s detected. Monitoring for situational awareness.",
				trackType,
			)
		}
	}

	// Default action
	return "track", 4, fmt.Sprintf(
		"Standard tracking recommended for %s %s.",
		classification, trackType,
	)
}

// determineConstraints sets operational constraints for the proposed action
func (a *PlannerAgent) determineConstraints(track *messages.CorrelatedTrack, actionType string) []string {
	constraints := []string{}

	switch actionType {
	case "engage":
		constraints = append(constraints,
			"Positive target identification required",
			"Rules of engagement must be satisfied",
			"Commander approval required",
			"Collateral damage assessment required",
		)
	case "intercept":
		constraints = append(constraints,
			"Verify target classification before intercept",
			"Maintain safe distance until identification",
			"Coordinate with command",
		)
	case "identify":
		constraints = append(constraints,
			"Use non-hostile identification methods first",
			"Maintain defensive posture",
		)
	case "track":
		constraints = append(constraints,
			"Maintain continuous track",
			"Report significant changes",
		)
	case "monitor":
		constraints = append(constraints,
			"Passive monitoring only",
			"No active interrogation",
		)
	}

	// Add classification-specific constraints
	if track.Classification == "friendly" {
		constraints = append(constraints, "Verify friendly IFF before any active measures")
	}

	return constraints
}

// determineExpiration sets how long the proposal is valid
func (a *PlannerAgent) determineExpiration(priority int) time.Duration {
	switch {
	case priority >= 9:
		return 1 * time.Minute // Critical - very short window
	case priority >= 7:
		return 2 * time.Minute // High priority
	case priority >= 5:
		return 5 * time.Minute // Medium priority
	default:
		return 15 * time.Minute // Low priority - longer consideration time
	}
}

// validateProposal checks the proposal against OPA policy
func (a *PlannerAgent) validateProposal(ctx context.Context, proposal *messages.ActionProposal, track *messages.CorrelatedTrack) (*opa.Decision, error) {
	// Use the OPA client's CheckProposal method
	decision, err := a.opaClient.CheckProposal(
		ctx,
		proposal,
		track,
		true,          // track exists
		[]interface{}{}, // no other pending proposals (simplified)
	)
	if err != nil {
		return nil, err
	}

	return decision, nil
}

func main() {
	// Configuration from environment
	cfg := agent.Config{
		ID:      getEnv("AGENT_ID", "planner-"+uuid.New().String()[:8]),
		Type:    agent.AgentTypePlanner,
		NATSUrl: getEnv("NATS_URL", "nats://localhost:4222"),
		OPAUrl:  getEnv("OPA_URL", "http://localhost:8181"),
		Secret:  []byte(getEnv("AGENT_SECRET", "planner-secret")),
	}

	// Create agent
	planner, err := NewPlannerAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create planner agent: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start metrics server
	go func() {
		metricsAddr := getEnv("METRICS_ADDR", ":9090")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(planner.Metrics(), promhttp.HandlerOpts{}))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			health := planner.Health()
			if health.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			json.NewEncoder(w).Encode(health)
		})
		planner.logger.Info().Str("addr", metricsAddr).Msg("Starting metrics server")
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			planner.logger.Error().Err(err).Msg("Metrics server error")
		}
	}()

	// Run agent
	go func() {
		if err := planner.Run(ctx); err != nil && err != context.Canceled {
			planner.logger.Error().Err(err).Msg("Planner agent error")
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	planner.logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := planner.Stop(shutdownCtx); err != nil {
		planner.logger.Error().Err(err).Msg("Error during shutdown")
	}

	planner.logger.Info().Msg("Planner agent stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
