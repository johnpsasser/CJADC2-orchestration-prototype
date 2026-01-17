// Authorizer Agent - Stores proposals in PostgreSQL and waits for human decisions
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agile-defense/cjadc2/pkg/agent"
	"github.com/agile-defense/cjadc2/pkg/messages"
	natsutil "github.com/agile-defense/cjadc2/pkg/nats"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// AuthorizerAgent stores proposals and waits for human decisions
type AuthorizerAgent struct {
	*agent.BaseAgent
	logger            zerolog.Logger
	consumer          jetstream.Consumer
	db                *pgxpool.Pool
	pendingProposals  map[string]*pendingProposal
	mu                sync.RWMutex
	proposalsStored   prometheus.Counter
	decisionsApproved prometheus.Counter
	decisionsDenied   prometheus.Counter
}

type pendingProposal struct {
	proposal   *messages.ActionProposal
	msg        jetstream.Msg
	receivedAt time.Time
}

// NewAuthorizerAgent creates a new authorizer agent
func NewAuthorizerAgent(cfg agent.Config) (*AuthorizerAgent, error) {
	base, err := agent.NewBaseAgent(cfg)
	if err != nil {
		return nil, err
	}

	// Additional metrics
	proposalsStored := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "authorizer_proposals_stored_total",
		Help: "Total number of proposals stored for authorization",
	})

	decisionsApproved := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "authorizer_decisions_approved_total",
		Help: "Total number of proposals approved",
	})

	decisionsDenied := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "authorizer_decisions_denied_total",
		Help: "Total number of proposals denied",
	})

	base.Metrics().MustRegister(proposalsStored, decisionsApproved, decisionsDenied)

	return &AuthorizerAgent{
		BaseAgent:         base,
		logger:            *base.Logger(),
		pendingProposals:  make(map[string]*pendingProposal),
		proposalsStored:   proposalsStored,
		decisionsApproved: decisionsApproved,
		decisionsDenied:   decisionsDenied,
	}, nil
}

// Run starts the authorizer agent
func (a *AuthorizerAgent) Run(ctx context.Context) error {
	// Start base agent (connects to NATS)
	if err := a.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base agent: %w", err)
	}

	// Connect to PostgreSQL
	if err := a.connectDB(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Ensure streams exist
	if err := natsutil.SetupStreams(ctx, a.JetStream()); err != nil {
		return fmt.Errorf("failed to setup streams: %w", err)
	}

	// Create consumer for proposals
	consumer, err := natsutil.SetupConsumer(ctx, a.JetStream(), "PROPOSALS", "authorizer")
	if err != nil {
		return fmt.Errorf("failed to setup consumer: %w", err)
	}
	a.consumer = consumer

	// Start expiration checker
	go a.expirationLoop(ctx)

	a.logger.Info().Msg("Authorizer agent started, consuming from PROPOSALS stream")

	// Start consuming messages
	return a.consumeMessages(ctx)
}

// connectDB establishes PostgreSQL connection
func (a *AuthorizerAgent) connectDB(ctx context.Context) error {
	dbURL := a.Config().DBUrl
	if dbURL == "" {
		dbURL = "postgres://cjadc2:devpassword@localhost:5432/cjadc2?sslmode=disable"
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("failed to parse database config: %w", err)
	}

	config.MaxConns = 10
	config.MinConns = 2
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
	a.logger.Info().Msg("Connected to PostgreSQL")
	return nil
}

// expirationLoop checks for expired proposals
func (a *AuthorizerAgent) expirationLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkExpiredProposals(ctx)
		}
	}
}

// checkExpiredProposals handles proposals that have expired
func (a *AuthorizerAgent) checkExpiredProposals(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for id, pending := range a.pendingProposals {
		if now.After(pending.proposal.ExpiresAt) {
			a.logger.Warn().
				Str("proposal_id", id).
				Str("action_type", pending.proposal.ActionType).
				Msg("Proposal expired without decision")

			// Update database
			_, err := a.db.Exec(ctx,
				"UPDATE proposals SET status = 'expired' WHERE proposal_id = $1",
				id,
			)
			if err != nil {
				a.logger.Error().Err(err).Str("proposal_id", id).Msg("Failed to update expired proposal")
			}

			// NAK the message so it won't be redelivered (exceeded max age)
			pending.msg.Term()
			delete(a.pendingProposals, id)
		}
	}
}

// consumeMessages processes proposal messages
func (a *AuthorizerAgent) consumeMessages(ctx context.Context) error {
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
				consumer, recreateErr := natsutil.SetupConsumer(ctx, a.JetStream(), "PROPOSALS", "authorizer")
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
			}
			// Note: We don't ACK here - we ACK when the human makes a decision
		}

		if msgs.Error() != nil && msgs.Error() != context.DeadlineExceeded {
			errStr := msgs.Error().Error()
			// Check if consumer was deleted and needs to be recreated
			if strings.Contains(errStr, "no responders") || strings.Contains(errStr, "consumer not found") || strings.Contains(errStr, "consumer deleted") {
				a.logger.Warn().Err(msgs.Error()).Msg("Consumer was deleted (batch error), recreating...")
				consumer, recreateErr := natsutil.SetupConsumer(ctx, a.JetStream(), "PROPOSALS", "authorizer")
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

// processMessage handles a single proposal message
func (a *AuthorizerAgent) processMessage(ctx context.Context, msg jetstream.Msg) error {
	start := time.Now()

	// Parse proposal
	var proposal messages.ActionProposal
	if err := json.Unmarshal(msg.Data(), &proposal); err != nil {
		msg.Term() // Don't retry malformed messages
		return fmt.Errorf("failed to unmarshal proposal: %w", err)
	}

	correlationID := proposal.Envelope.CorrelationID
	if correlationID == "" {
		correlationID = proposal.Envelope.MessageID
	}

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("proposal_id", proposal.ProposalID).
		Str("track_id", proposal.TrackID).
		Str("action_type", proposal.ActionType).
		Int("priority", proposal.Priority).
		Msg("Processing proposal")

	// Check if there's already a pending proposal for this track
	var existingProposalID string
	var existingHitCount int
	err := a.db.QueryRow(ctx,
		"SELECT proposal_id, hit_count FROM proposals WHERE track_id = $1 AND status = 'pending'",
		proposal.TrackID,
	).Scan(&existingProposalID, &existingHitCount)

	constraintsJSON, _ := json.Marshal(proposal.Constraints)
	trackDataJSON, _ := json.Marshal(proposal.Track)
	policyJSON, _ := json.Marshal(proposal.PolicyDecision)
	now := time.Now().UTC()

	if err == nil {
		// Existing pending proposal for this track - UPDATE it
		newHitCount := existingHitCount + 1

		// Take the higher priority, update track data, increment hit count
		_, err = a.db.Exec(ctx, `
			UPDATE proposals SET
				track_data = $1,
				priority = GREATEST(priority, $2),
				threat_level = $3,
				action_type = CASE WHEN $2 > priority THEN $4 ELSE action_type END,
				rationale = CASE WHEN $2 > priority THEN $5 ELSE rationale END,
				constraints = CASE WHEN $2 > priority THEN $6 ELSE constraints END,
				policy_decision = $7,
				hit_count = $8,
				last_hit_at = $9,
				expires_at = GREATEST(expires_at, $10),
				updated_at = $9
			WHERE proposal_id = $11
		`,
			trackDataJSON,
			proposal.Priority,
			proposal.ThreatLevel,
			proposal.ActionType,
			proposal.Rationale,
			constraintsJSON,
			policyJSON,
			newHitCount,
			now,
			proposal.ExpiresAt,
			existingProposalID,
		)
		if err != nil {
			return fmt.Errorf("failed to update proposal: %w", err)
		}

		// ACK immediately - we've merged this into existing proposal
		msg.Ack()

		duration := time.Since(start)
		a.RecordMessage("success", "proposal")
		a.RecordLatency("proposal", duration)

		a.logger.Info().
			Str("correlation_id", correlationID).
			Str("existing_proposal_id", existingProposalID).
			Str("track_id", proposal.TrackID).
			Int("hit_count", newHitCount).
			Dur("latency_ms", duration).
			Msg("Merged into existing proposal (de-duplicated)")

		return nil
	} else if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to check existing proposal: %w", err)
	}

	// Check if there's a recent decision for this track (cooldown period)
	// This prevents new proposals from being created immediately after a decision
	var recentDecisionID string
	var recentDecisionApproved bool
	var recentDecisionAt time.Time
	err = a.db.QueryRow(ctx,
		`SELECT decision_id, approved, approved_at FROM decisions
		 WHERE track_id = $1 AND approved_at > NOW() - INTERVAL '5 minutes'
		 ORDER BY approved_at DESC LIMIT 1`,
		proposal.TrackID,
	).Scan(&recentDecisionID, &recentDecisionApproved, &recentDecisionAt)

	if err == nil {
		// Recent decision exists - skip creating new proposal (cooldown period)
		msg.Ack()

		duration := time.Since(start)
		a.RecordMessage("success", "proposal")
		a.RecordLatency("proposal", duration)

		a.logger.Info().
			Str("correlation_id", correlationID).
			Str("track_id", proposal.TrackID).
			Str("recent_decision_id", recentDecisionID).
			Bool("was_approved", recentDecisionApproved).
			Time("decided_at", recentDecisionAt).
			Dur("latency_ms", duration).
			Msg("Skipped proposal - recent decision exists (cooldown period)")

		return nil
	} else if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to check recent decisions: %w", err)
	}

	// No existing pending proposal or recent decision for this track - INSERT new one
	_, err = a.db.Exec(ctx, `
		INSERT INTO proposals (
			proposal_id, track_id, action_type, priority, threat_level,
			rationale, constraints, track_data, policy_decision, expires_at,
			status, correlation_id, hit_count, last_hit_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', $11, 1, $12)
	`,
		proposal.ProposalID,
		proposal.TrackID,
		proposal.ActionType,
		proposal.Priority,
		proposal.ThreatLevel,
		proposal.Rationale,
		constraintsJSON,
		trackDataJSON,
		policyJSON,
		proposal.ExpiresAt,
		correlationID,
		now,
	)
	if err != nil {
		// Check if it's a unique constraint violation (race condition - another proposal was just inserted)
		if strings.Contains(err.Error(), "idx_proposals_track_pending_unique") {
			// Retry by updating the existing proposal
			a.logger.Debug().
				Str("track_id", proposal.TrackID).
				Msg("Race condition detected, retrying as update")
			msg.Nak() // Will be redelivered and handled as update
			return nil
		}
		return fmt.Errorf("failed to store proposal: %w", err)
	}

	// Store in pending map for later acknowledgment
	a.mu.Lock()
	a.pendingProposals[proposal.ProposalID] = &pendingProposal{
		proposal:   &proposal,
		msg:        msg,
		receivedAt: time.Now(),
	}
	a.mu.Unlock()

	duration := time.Since(start)
	a.RecordMessage("success", "proposal")
	a.RecordLatency("proposal", duration)
	a.proposalsStored.Inc()

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("proposal_id", proposal.ProposalID).
		Str("track_id", proposal.TrackID).
		Dur("latency_ms", duration).
		Msg("New proposal stored, awaiting human decision")

	return nil
}

// ProcessDecision handles a human decision on a proposal (called via API)
func (a *AuthorizerAgent) ProcessDecision(ctx context.Context, proposalID string, approved bool, approvedBy, reason string, conditions []string) error {
	a.mu.Lock()
	pending, exists := a.pendingProposals[proposalID]
	if exists {
		delete(a.pendingProposals, proposalID)
	}
	a.mu.Unlock()

	// Get proposal from database if not in memory
	var proposal messages.ActionProposal
	if pending != nil {
		proposal = *pending.proposal
	} else {
		var trackData, constraintsData, policyData []byte
		var correlationID string
		err := a.db.QueryRow(ctx, `
			SELECT proposal_id, track_id, action_type, priority, threat_level,
				   rationale, constraints, track_data, policy_decision, expires_at, correlation_id
			FROM proposals WHERE proposal_id = $1
		`, proposalID).Scan(
			&proposal.ProposalID,
			&proposal.TrackID,
			&proposal.ActionType,
			&proposal.Priority,
			&proposal.ThreatLevel,
			&proposal.Rationale,
			&constraintsData,
			&trackData,
			&policyData,
			&proposal.ExpiresAt,
			&correlationID,
		)
		if err != nil {
			return fmt.Errorf("proposal not found: %w", err)
		}

		json.Unmarshal(constraintsData, &proposal.Constraints)
		json.Unmarshal(trackData, &proposal.Track)
		json.Unmarshal(policyData, &proposal.PolicyDecision)
		proposal.Envelope.CorrelationID = correlationID
	}

	// Create decision
	decision := messages.NewDecision(&proposal, a.ID())
	decision.DecisionID = uuid.New().String()
	decision.Approved = approved
	decision.ApprovedBy = approvedBy
	decision.ApprovedAt = time.Now().UTC()
	decision.Reason = reason
	decision.Conditions = conditions

	// Store decision in database
	conditionsJSON, _ := json.Marshal(conditions)
	_, err := a.db.Exec(ctx, `
		INSERT INTO decisions (
			decision_id, proposal_id, approved, approved_by, approved_at,
			reason, conditions, action_type, track_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		decision.DecisionID,
		proposal.ProposalID,
		approved,
		approvedBy,
		decision.ApprovedAt,
		reason,
		conditionsJSON,
		proposal.ActionType,
		proposal.TrackID,
	)
	if err != nil {
		return fmt.Errorf("failed to store decision: %w", err)
	}

	// Update proposal status
	status := "approved"
	if !approved {
		status = "denied"
	}
	_, err = a.db.Exec(ctx,
		"UPDATE proposals SET status = $1 WHERE proposal_id = $2",
		status, proposal.ProposalID,
	)
	if err != nil {
		return fmt.Errorf("failed to update proposal status: %w", err)
	}

	// Publish decision to DECISIONS stream
	subject := decision.Subject()
	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("failed to marshal decision: %w", err)
	}

	_, err = a.JetStream().Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish decision: %w", err)
	}

	// ACK the original message if we have it
	if pending != nil {
		pending.msg.Ack()
	}

	// Update metrics
	if approved {
		a.decisionsApproved.Inc()
	} else {
		a.decisionsDenied.Inc()
	}

	a.logger.Info().
		Str("decision_id", decision.DecisionID).
		Str("proposal_id", proposal.ProposalID).
		Bool("approved", approved).
		Str("approved_by", approvedBy).
		Str("subject", subject).
		Msg("Decision published")

	return nil
}

// GetPendingProposals returns all pending proposals for the UI
func (a *AuthorizerAgent) GetPendingProposals(ctx context.Context) ([]map[string]interface{}, error) {
	rows, err := a.db.Query(ctx, `
		SELECT proposal_id, track_id, action_type, priority, threat_level,
			   rationale, constraints, track_data, policy_decision, expires_at,
			   created_at, correlation_id, hit_count, last_hit_at
		FROM proposals
		WHERE status = 'pending' AND expires_at > NOW()
		ORDER BY priority DESC, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query proposals: %w", err)
	}
	defer rows.Close()

	var proposals []map[string]interface{}
	for rows.Next() {
		var (
			proposalID, trackID, actionType, threatLevel, rationale, correlationID string
			priority, hitCount                                                      int
			constraints, trackData, policyDecision                                  []byte
			expiresAt, createdAt, lastHitAt                                         time.Time
		)

		if err := rows.Scan(
			&proposalID, &trackID, &actionType, &priority, &threatLevel,
			&rationale, &constraints, &trackData, &policyDecision, &expiresAt,
			&createdAt, &correlationID, &hitCount, &lastHitAt,
		); err != nil {
			continue
		}

		var constraintsList []string
		var track map[string]interface{}
		var policy map[string]interface{}
		json.Unmarshal(constraints, &constraintsList)
		json.Unmarshal(trackData, &track)
		json.Unmarshal(policyDecision, &policy)

		proposals = append(proposals, map[string]interface{}{
			"proposal_id":     proposalID,
			"track_id":        trackID,
			"action_type":     actionType,
			"priority":        priority,
			"threat_level":    threatLevel,
			"rationale":       rationale,
			"constraints":     constraintsList,
			"track":           track,
			"policy_decision": policy,
			"expires_at":      expiresAt,
			"created_at":      createdAt,
			"correlation_id":  correlationID,
			"hit_count":       hitCount,
			"last_hit_at":     lastHitAt,
		})
	}

	return proposals, nil
}

func main() {
	// Configuration from environment
	cfg := agent.Config{
		ID:      getEnv("AGENT_ID", "authorizer-"+uuid.New().String()[:8]),
		Type:    agent.AgentTypeAuthorizer,
		NATSUrl: getEnv("NATS_URL", "nats://localhost:4222"),
		OPAUrl:  getEnv("OPA_URL", "http://localhost:8181"),
		DBUrl:   getEnv("DATABASE_URL", "postgres://cjadc2:devpassword@localhost:5432/cjadc2?sslmode=disable"),
		Secret:  []byte(getEnv("AGENT_SECRET", "authorizer-secret")),
	}

	// Create agent
	authorizer, err := NewAuthorizerAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create authorizer agent: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server (metrics + API for decisions)
	go func() {
		metricsAddr := getEnv("METRICS_ADDR", ":9090")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(authorizer.Metrics(), promhttp.HandlerOpts{}))

		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			health := authorizer.Health()
			if health.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			json.NewEncoder(w).Encode(health)
		})

		// API endpoint for getting pending proposals
		mux.HandleFunc("/api/proposals", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			proposals, err := authorizer.GetPendingProposals(r.Context())
			if err != nil {
				authorizer.logger.Error().Err(err).Msg("Failed to get proposals")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(proposals)
		})

		// API endpoint for submitting decisions
		mux.HandleFunc("/api/decisions", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			var req struct {
				ProposalID string   `json:"proposal_id"`
				Approved   bool     `json:"approved"`
				ApprovedBy string   `json:"approved_by"`
				Reason     string   `json:"reason"`
				Conditions []string `json:"conditions"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}

			if req.ProposalID == "" {
				http.Error(w, "proposal_id is required", http.StatusBadRequest)
				return
			}

			if req.ApprovedBy == "" {
				http.Error(w, "approved_by is required", http.StatusBadRequest)
				return
			}

			if err := authorizer.ProcessDecision(
				r.Context(),
				req.ProposalID,
				req.Approved,
				req.ApprovedBy,
				req.Reason,
				req.Conditions,
			); err != nil {
				authorizer.logger.Error().Err(err).Msg("Failed to process decision")
				http.Error(w, fmt.Sprintf("Failed to process decision: %v", err), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		})

		authorizer.logger.Info().Str("addr", metricsAddr).Msg("Starting HTTP server")
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			authorizer.logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Run agent
	go func() {
		if err := authorizer.Run(ctx); err != nil && err != context.Canceled {
			authorizer.logger.Error().Err(err).Msg("Authorizer agent error")
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	authorizer.logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := authorizer.Stop(shutdownCtx); err != nil {
		authorizer.logger.Error().Err(err).Msg("Error during shutdown")
	}

	if authorizer.db != nil {
		authorizer.db.Close()
	}

	authorizer.logger.Info().Msg("Authorizer agent stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
