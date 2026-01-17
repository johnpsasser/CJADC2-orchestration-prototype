// Planner Agent - Generates action proposals based on correlated tracks and threat levels
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
	"github.com/agile-defense/cjadc2/pkg/opa"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
	db               *pgxpool.Pool
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

	// Connect to PostgreSQL for intervention rules
	if err := a.connectDB(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
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
			// Check if consumer was deleted and needs to be recreated
			errStr := err.Error()
			if strings.Contains(errStr, "no responders") || strings.Contains(errStr, "consumer not found") || strings.Contains(errStr, "consumer deleted") {
				a.logger.Warn().Err(err).Msg("Consumer was deleted, recreating...")
				consumer, recreateErr := natsutil.SetupConsumer(ctx, a.JetStream(), "TRACKS", "planner")
				if recreateErr != nil {
					a.logger.Error().Err(recreateErr).Msg("Failed to recreate consumer")
					a.RecordError("consumer_recreate_error")
					time.Sleep(time.Second)
					continue
				}
				a.consumer = consumer
				a.logger.Info().Msg("Consumer recreated successfully")
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
			errStr := msgs.Error().Error()
			// Check if consumer was deleted and needs to be recreated
			if strings.Contains(errStr, "no responders") || strings.Contains(errStr, "consumer not found") || strings.Contains(errStr, "consumer deleted") {
				a.logger.Warn().Err(msgs.Error()).Msg("Consumer was deleted (batch error), recreating...")
				consumer, recreateErr := natsutil.SetupConsumer(ctx, a.JetStream(), "TRACKS", "planner")
				if recreateErr != nil {
					a.logger.Error().Err(recreateErr).Msg("Failed to recreate consumer")
					a.RecordError("consumer_recreate_error")
				} else {
					a.consumer = consumer
					a.logger.Info().Msg("Consumer recreated successfully")
				}
				continue
			}
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

	// Determine action based on track characteristics
	actionType, priority, rationale := a.determineAction(&track)

	// Check if this action requires human-in-the-loop approval
	if !a.requiresHumanApproval(actionType, priority, track.Classification, track.ThreatLevel) {
		// Passive action - log and skip proposal creation
		duration := time.Since(start)
		a.RecordMessage("success", "correlated_track")
		a.RecordLatency("correlated_track", duration)

		a.logger.Info().
			Str("correlation_id", correlationID).
			Str("track_id", track.TrackID).
			Str("action_type", actionType).
			Int("priority", priority).
			Str("rationale", rationale).
			Dur("latency_ms", duration).
			Msg("Passive action - no proposal required (auto-approved)")

		return nil
	}

	// Generate action proposal for HITL review
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
		Bool("requires_hitl", true).
		Msg("Proposal generated - requires human approval")

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
		Msg("Published action proposal for HITL review")

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
		return 10 * time.Minute // Critical - short window but enough time for review
	case priority >= 7:
		return 15 * time.Minute // High priority
	case priority >= 5:
		return 30 * time.Minute // Medium priority
	default:
		return 60 * time.Minute // Low priority - longer consideration time
	}
}

// connectDB establishes PostgreSQL connection
func (a *PlannerAgent) connectDB(ctx context.Context) error {
	dbURL := a.Config().DBUrl
	if dbURL == "" {
		dbURL = "postgres://cjadc2:devpassword@localhost:5432/cjadc2?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database config: %w", err)
	}

	config.MaxConns = 5
	config.MinConns = 1
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	a.db = pool
	a.logger.Info().Msg("Connected to PostgreSQL for intervention rules")
	return nil
}

// interventionRule represents a rule from the database
type interventionRule struct {
	RuleID           string
	Name             string
	ActionTypes      []string
	ThreatLevels     []string
	Classifications  []string
	TrackTypes       []string
	MinPriority      *int
	MaxPriority      *int
	RequiresApproval bool
	AutoApprove      bool
	EvaluationOrder  int
}

// getMatchingInterventionRules queries the database for rules that match the given criteria
func (a *PlannerAgent) getMatchingInterventionRules(ctx context.Context, actionType, classification, threatLevel string, priority int) ([]interventionRule, error) {
	query := `
		SELECT rule_id, name, action_types, threat_levels, classifications, track_types,
		       min_priority, max_priority, requires_approval, auto_approve, evaluation_order
		FROM intervention_rules
		WHERE enabled = true
		  AND (cardinality(action_types) = 0 OR $1 = ANY(action_types))
		  AND (cardinality(classifications) = 0 OR $2 = ANY(classifications))
		  AND (cardinality(threat_levels) = 0 OR $3 = ANY(threat_levels))
		  AND (min_priority IS NULL OR $4 >= min_priority)
		  AND (max_priority IS NULL OR $4 <= max_priority)
		ORDER BY evaluation_order ASC
	`

	rows, err := a.db.Query(ctx, query, actionType, classification, threatLevel, priority)
	if err != nil {
		return nil, fmt.Errorf("failed to query intervention rules: %w", err)
	}
	defer rows.Close()

	var rules []interventionRule
	for rows.Next() {
		var rule interventionRule
		err := rows.Scan(
			&rule.RuleID,
			&rule.Name,
			&rule.ActionTypes,
			&rule.ThreatLevels,
			&rule.Classifications,
			&rule.TrackTypes,
			&rule.MinPriority,
			&rule.MaxPriority,
			&rule.RequiresApproval,
			&rule.AutoApprove,
			&rule.EvaluationOrder,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan intervention rule: %w", err)
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

// requiresHumanApproval determines if an action needs human-in-the-loop approval
// Uses configurable intervention rules from the database
// Falls back to hardcoded defaults if database is unavailable
func (a *PlannerAgent) requiresHumanApproval(actionType string, priority int, classification, threatLevel string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Query matching intervention rules from database
	rules, err := a.getMatchingInterventionRules(ctx, actionType, classification, threatLevel, priority)
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to query intervention rules, using fallback logic")
		return a.fallbackRequiresHumanApproval(actionType, priority)
	}

	// If we have matching rules, use the first one (highest priority by evaluation_order)
	if len(rules) > 0 {
		rule := rules[0]
		a.logger.Debug().
			Str("rule_id", rule.RuleID).
			Str("rule_name", rule.Name).
			Bool("requires_approval", rule.RequiresApproval).
			Bool("auto_approve", rule.AutoApprove).
			Msg("Using intervention rule")

		// If auto_approve is true, no human approval needed
		if rule.AutoApprove {
			return false
		}
		return rule.RequiresApproval
	}

	// No matching rules found - use fallback logic for safety
	a.logger.Debug().Msg("No matching intervention rules found, using fallback logic")
	return a.fallbackRequiresHumanApproval(actionType, priority)
}

// fallbackRequiresHumanApproval provides default behavior when database is unavailable
// Based on CJADC2 doctrine:
// - Kinetic/active actions (engage, intercept) ALWAYS require HITL
// - Identification actions require HITL when priority is high
// - Passive actions (track, monitor, ignore) do NOT require HITL
func (a *PlannerAgent) fallbackRequiresHumanApproval(actionType string, priority int) bool {
	switch actionType {
	case "engage":
		// Kinetic action - ALWAYS requires human approval
		return true
	case "intercept":
		// Active engagement - ALWAYS requires human approval
		return true
	case "identify":
		// Identification - requires approval only for high priority (>=6)
		return priority >= 6
	case "track", "monitor", "ignore":
		// Passive observation - does NOT require human approval
		return false
	default:
		// Unknown action types require approval for safety
		return true
	}
}

// validateProposal checks the proposal against OPA policy
func (a *PlannerAgent) validateProposal(ctx context.Context, proposal *messages.ActionProposal, track *messages.CorrelatedTrack) (*opa.Decision, error) {
	// Use the OPA client's CheckProposal method
	decision, err := a.opaClient.CheckProposal(
		ctx,
		proposal,
		track,
		true,            // track exists
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
		DBUrl:   getEnv("POSTGRES_URL", "postgres://cjadc2:devpassword@localhost:5432/cjadc2?sslmode=disable"),
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
