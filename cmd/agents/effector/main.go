// Effector Agent - Executes approved decisions with idempotency and OPA validation
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// EffectorAgent executes approved decisions
type EffectorAgent struct {
	*agent.BaseAgent
	logger            zerolog.Logger
	consumer          jetstream.Consumer
	db                *pgxpool.Pool
	opaClient         *opa.Client
	effectsExecuted   prometheus.Counter
	effectsFailed     prometheus.Counter
	effectsIdempotent prometheus.Counter
}

// NewEffectorAgent creates a new effector agent
func NewEffectorAgent(cfg agent.Config) (*EffectorAgent, error) {
	base, err := agent.NewBaseAgent(cfg)
	if err != nil {
		return nil, err
	}

	// Additional metrics
	effectsExecuted := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "effector_effects_executed_total",
		Help: "Total number of effects executed",
	})

	effectsFailed := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "effector_effects_failed_total",
		Help: "Total number of effects that failed",
	})

	effectsIdempotent := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "effector_effects_idempotent_total",
		Help: "Total number of idempotent effect requests (already executed)",
	})

	base.Metrics().MustRegister(effectsExecuted, effectsFailed, effectsIdempotent)

	return &EffectorAgent{
		BaseAgent:         base,
		logger:            *base.Logger(),
		opaClient:         opa.NewClient(cfg.OPAUrl),
		effectsExecuted:   effectsExecuted,
		effectsFailed:     effectsFailed,
		effectsIdempotent: effectsIdempotent,
	}, nil
}

// Run starts the effector agent
func (a *EffectorAgent) Run(ctx context.Context) error {
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

	// Create consumer for approved decisions
	consumer, err := natsutil.SetupConsumer(ctx, a.JetStream(), "DECISIONS", "effector")
	if err != nil {
		return fmt.Errorf("failed to setup consumer: %w", err)
	}
	a.consumer = consumer

	a.logger.Info().Msg("Effector agent started, consuming from DECISIONS stream")

	// Start consuming messages
	return a.consumeMessages(ctx)
}

// connectDB establishes PostgreSQL connection
func (a *EffectorAgent) connectDB(ctx context.Context) error {
	dbURL := a.Config().DBUrl
	if dbURL == "" {
		dbURL = "postgres://cjadc2:cjadc2@localhost:5432/cjadc2?sslmode=disable"
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

// consumeMessages processes approved decision messages
func (a *EffectorAgent) consumeMessages(ctx context.Context) error {
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

// processMessage handles a single approved decision message
func (a *EffectorAgent) processMessage(ctx context.Context, msg jetstream.Msg) error {
	start := time.Now()

	// Parse decision
	var decision messages.Decision
	if err := json.Unmarshal(msg.Data(), &decision); err != nil {
		msg.Term() // Don't retry malformed messages
		return fmt.Errorf("failed to unmarshal decision: %w", err)
	}

	// Only process approved decisions
	if !decision.Approved {
		a.logger.Info().
			Str("decision_id", decision.DecisionID).
			Msg("Skipping denied decision")
		return nil
	}

	correlationID := decision.Envelope.CorrelationID
	if correlationID == "" {
		correlationID = decision.Envelope.MessageID
	}

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("decision_id", decision.DecisionID).
		Str("action_type", decision.ActionType).
		Str("approved_by", decision.ApprovedBy).
		Msg("Processing approved decision")

	// Generate idempotency key
	idempotentKey := fmt.Sprintf("%s-%s-%s", decision.DecisionID, decision.ProposalID, decision.ActionType)

	// Check idempotency - has this effect already been executed?
	alreadyExecuted, err := a.checkIdempotency(ctx, idempotentKey)
	if err != nil {
		return fmt.Errorf("failed to check idempotency: %w", err)
	}

	if alreadyExecuted {
		a.logger.Info().
			Str("correlation_id", correlationID).
			Str("idempotent_key", idempotentKey).
			Msg("Effect already executed (idempotent)")
		a.effectsIdempotent.Inc()
		return nil
	}

	// Get proposal details for OPA validation
	proposal, err := a.getProposal(ctx, decision.ProposalID)
	if err != nil {
		a.logger.Warn().
			Err(err).
			Str("proposal_id", decision.ProposalID).
			Msg("Could not retrieve proposal, proceeding with limited validation")
		proposal = nil
	}

	// Validate with OPA policy - requires human approval check
	opaDecision, err := a.validateEffect(ctx, &decision, proposal)
	if err != nil {
		a.logger.Warn().
			Err(err).
			Str("correlation_id", correlationID).
			Msg("OPA validation failed, proceeding with warning")
		// Continue but log the warning
	} else if !opaDecision.Allowed {
		// OPA explicitly denied - this should not happen for approved decisions
		// but we handle it for safety
		a.logger.Error().
			Str("correlation_id", correlationID).
			Strs("reasons", opaDecision.Reasons).
			Msg("OPA denied effect execution")

		// Record failed effect
		effectLog := a.createEffectLog(&decision, correlationID, idempotentKey, "failed", "OPA policy denied execution")
		if err := a.storeEffect(ctx, effectLog); err != nil {
			a.logger.Error().Err(err).Msg("Failed to store failed effect")
		}
		a.publishEffectLog(ctx, effectLog)
		a.effectsFailed.Inc()

		return nil // Don't retry - policy denied
	}

	// Execute the effect (simulated)
	result, err := a.executeEffect(ctx, &decision, correlationID)
	if err != nil {
		a.logger.Error().
			Err(err).
			Str("correlation_id", correlationID).
			Msg("Effect execution failed")

		// Record failed effect
		effectLog := a.createEffectLog(&decision, correlationID, idempotentKey, "failed", err.Error())
		if storeErr := a.storeEffect(ctx, effectLog); storeErr != nil {
			a.logger.Error().Err(storeErr).Msg("Failed to store failed effect")
		}
		a.publishEffectLog(ctx, effectLog)
		a.effectsFailed.Inc()

		return err // Retry on execution failure
	}

	// Record successful effect
	effectLog := a.createEffectLog(&decision, correlationID, idempotentKey, "executed", result)
	if err := a.storeEffect(ctx, effectLog); err != nil {
		return fmt.Errorf("failed to store effect: %w", err)
	}

	// Publish effect log
	a.publishEffectLog(ctx, effectLog)

	duration := time.Since(start)
	a.RecordMessage("success", "decision")
	a.RecordLatency("decision", duration)
	a.effectsExecuted.Inc()

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("effect_id", effectLog.EffectID).
		Str("result", result).
		Dur("latency_ms", duration).
		Msg("Effect executed successfully")

	return nil
}

// checkIdempotency checks if an effect has already been executed
func (a *EffectorAgent) checkIdempotency(ctx context.Context, idempotentKey string) (bool, error) {
	var exists bool
	err := a.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM effects WHERE idempotent_key = $1)",
		idempotentKey,
	).Scan(&exists)

	if err != nil {
		return false, err
	}

	return exists, nil
}

// getProposal retrieves the original proposal from the database
func (a *EffectorAgent) getProposal(ctx context.Context, proposalID string) (map[string]interface{}, error) {
	var (
		trackID, actionType, threatLevel, rationale string
		priority                                    int
		trackData, policyData                       []byte
		expiresAt                                   time.Time
	)

	err := a.db.QueryRow(ctx, `
		SELECT track_id, action_type, priority, threat_level, rationale,
			   track_data, policy_decision, expires_at
		FROM proposals WHERE proposal_id = $1
	`, proposalID).Scan(
		&trackID, &actionType, &priority, &threatLevel, &rationale,
		&trackData, &policyData, &expiresAt,
	)

	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("proposal not found")
	}
	if err != nil {
		return nil, err
	}

	var track, policy map[string]interface{}
	json.Unmarshal(trackData, &track)
	json.Unmarshal(policyData, &policy)

	return map[string]interface{}{
		"proposal_id":     proposalID,
		"track_id":        trackID,
		"action_type":     actionType,
		"priority":        priority,
		"threat_level":    threatLevel,
		"rationale":       rationale,
		"track":           track,
		"policy_decision": policy,
		"expires_at":      expiresAt.Format(time.RFC3339),
	}, nil
}

// validateEffect checks with OPA if the effect can be released
func (a *EffectorAgent) validateEffect(ctx context.Context, decision *messages.Decision, proposal map[string]interface{}) (*opa.Decision, error) {
	// Get idempotency check from database
	alreadyExecuted, _ := a.checkIdempotency(ctx, fmt.Sprintf("%s-%s-%s", decision.DecisionID, decision.ProposalID, decision.ActionType))

	return a.opaClient.CheckEffectRelease(
		ctx,
		decision,
		proposal,
		decision.ActionType,
		alreadyExecuted,
	)
}

// executeEffect performs the simulated effect execution
func (a *EffectorAgent) executeEffect(ctx context.Context, decision *messages.Decision, correlationID string) (string, error) {
	// This is a SIMULATED effect execution
	// In a real system, this would interface with actual command and control systems

	actionType := decision.ActionType
	trackID := decision.TrackID
	approvedBy := decision.ApprovedBy

	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("action_type", actionType).
		Str("track_id", trackID).
		Str("approved_by", approvedBy).
		Msg("SIMULATED: Executing effect")

	// Simulate different execution times based on action type
	var executionTime time.Duration
	switch actionType {
	case "engage":
		executionTime = 100 * time.Millisecond
	case "intercept":
		executionTime = 75 * time.Millisecond
	case "identify":
		executionTime = 50 * time.Millisecond
	case "track":
		executionTime = 25 * time.Millisecond
	case "monitor":
		executionTime = 10 * time.Millisecond
	default:
		executionTime = 25 * time.Millisecond
	}

	// Simulate execution
	time.Sleep(executionTime)

	// Generate result message
	result := fmt.Sprintf("SIMULATED: Action '%s' executed against track '%s'. Approved by: %s. Execution time: %v",
		actionType, trackID, approvedBy, executionTime)

	// Log the simulated effect for audit
	a.logger.Info().
		Str("correlation_id", correlationID).
		Str("action_type", actionType).
		Str("track_id", trackID).
		Dur("execution_time", executionTime).
		Msg("SIMULATED: Effect execution completed")

	return result, nil
}

// createEffectLog creates an effect log message
func (a *EffectorAgent) createEffectLog(decision *messages.Decision, correlationID, idempotentKey, status, result string) *messages.EffectLog {
	effectLog := messages.NewEffectLog(decision, a.ID())
	effectLog.EffectID = uuid.New().String()
	effectLog.Status = status
	effectLog.Result = result
	effectLog.IdempotentKey = idempotentKey
	effectLog.Envelope.CorrelationID = correlationID

	return effectLog
}

// storeEffect saves the effect log to the database
func (a *EffectorAgent) storeEffect(ctx context.Context, effectLog *messages.EffectLog) error {
	_, err := a.db.Exec(ctx, `
		INSERT INTO effects (
			effect_id, message_id, correlation_id, decision_id, proposal_id,
			track_id, action_type, status, result, idempotent_key, executed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (idempotent_key) DO NOTHING
	`,
		effectLog.EffectID,
		effectLog.Envelope.MessageID,
		effectLog.Envelope.CorrelationID,
		effectLog.DecisionID,
		effectLog.ProposalID,
		effectLog.TrackID,
		effectLog.ActionType,
		effectLog.Status,
		effectLog.Result,
		effectLog.IdempotentKey,
		effectLog.ExecutedAt,
	)

	return err
}

// publishEffectLog publishes the effect log to NATS
func (a *EffectorAgent) publishEffectLog(ctx context.Context, effectLog *messages.EffectLog) error {
	subject := effectLog.Subject()
	data, err := json.Marshal(effectLog)
	if err != nil {
		return fmt.Errorf("failed to marshal effect log: %w", err)
	}

	_, err = a.JetStream().Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish effect log: %w", err)
	}

	a.logger.Info().
		Str("subject", subject).
		Str("effect_id", effectLog.EffectID).
		Str("status", effectLog.Status).
		Msg("Published effect log")

	return nil
}

// GetEffects returns all effects for the UI/API
func (a *EffectorAgent) GetEffects(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := a.db.Query(ctx, `
		SELECT effect_id, decision_id, proposal_id, track_id, action_type,
			   status, result, idempotent_key, executed_at, correlation_id
		FROM effects
		ORDER BY executed_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query effects: %w", err)
	}
	defer rows.Close()

	var effects []map[string]interface{}
	for rows.Next() {
		var (
			effectID, decisionID, proposalID, trackID, actionType string
			status, result, idempotentKey, correlationID          string
			executedAt                                            time.Time
		)

		if err := rows.Scan(
			&effectID, &decisionID, &proposalID, &trackID, &actionType,
			&status, &result, &idempotentKey, &executedAt, &correlationID,
		); err != nil {
			continue
		}

		effects = append(effects, map[string]interface{}{
			"effect_id":      effectID,
			"decision_id":    decisionID,
			"proposal_id":    proposalID,
			"track_id":       trackID,
			"action_type":    actionType,
			"status":         status,
			"result":         result,
			"idempotent_key": idempotentKey,
			"executed_at":    executedAt,
			"correlation_id": correlationID,
		})
	}

	return effects, nil
}

func main() {
	// Configuration from environment
	cfg := agent.Config{
		ID:      getEnv("AGENT_ID", "effector-"+uuid.New().String()[:8]),
		Type:    agent.AgentTypeEffector,
		NATSUrl: getEnv("NATS_URL", "nats://localhost:4222"),
		OPAUrl:  getEnv("OPA_URL", "http://localhost:8181"),
		DBUrl:   getEnv("DATABASE_URL", "postgres://cjadc2:cjadc2@localhost:5432/cjadc2?sslmode=disable"),
		Secret:  []byte(getEnv("AGENT_SECRET", "effector-secret")),
	}

	// Create agent
	effector, err := NewEffectorAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create effector agent: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server (metrics + API)
	go func() {
		metricsAddr := getEnv("METRICS_ADDR", ":9090")
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(effector.Metrics(), promhttp.HandlerOpts{}))

		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			health := effector.Health()
			if health.Healthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
			json.NewEncoder(w).Encode(health)
		})

		// API endpoint for getting effects
		mux.HandleFunc("/api/effects", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			effects, err := effector.GetEffects(r.Context(), 100)
			if err != nil {
				effector.logger.Error().Err(err).Msg("Failed to get effects")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(effects)
		})

		effector.logger.Info().Str("addr", metricsAddr).Msg("Starting HTTP server")
		if err := http.ListenAndServe(metricsAddr, mux); err != nil {
			effector.logger.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Run agent
	go func() {
		if err := effector.Run(ctx); err != nil && err != context.Canceled {
			effector.logger.Error().Err(err).Msg("Effector agent error")
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	effector.logger.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := effector.Stop(shutdownCtx); err != nil {
		effector.logger.Error().Err(err).Msg("Error during shutdown")
	}

	if effector.db != nil {
		effector.db.Close()
	}

	effector.logger.Info().Msg("Effector agent stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
