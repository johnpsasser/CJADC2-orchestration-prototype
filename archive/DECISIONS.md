# Technical Decisions

This document captures the key technical decisions made during the design and implementation of the CJADC2 platform, including the rationale and trade-offs considered.

## Message Broker: NATS JetStream

### Decision
Use NATS JetStream as the event streaming backbone instead of Apache Kafka or RabbitMQ.

### Rationale

**Why not Kafka:**
- Kafka excels at extremely high throughput (millions of messages/second) but introduces significant operational complexity
- Requires ZooKeeper/KRaft coordination, partition management, and careful tuning
- Schema registry, offset management, and consumer groups add cognitive overhead
- Overkill for a demonstration platform where operational simplicity matters

**Why not RabbitMQ:**
- Excellent for traditional message queuing but lacks native stream semantics
- Limited replay capabilities compared to log-based systems
- Would require additional infrastructure for the event-sourcing patterns we need

**Why NATS JetStream:**
- Single binary deployment with minimal configuration
- Native support for streams with replay, acknowledgments, and exactly-once delivery
- Built-in subject-based routing matches our hierarchical topic design (detect.>, track.>, etc.)
- JetStream provides persistence with configurable retention policies
- Work queue semantics for the proposal stream ensures single-consumer processing
- Excellent Go client with idiomatic API
- Sub-millisecond latency suitable for real-time decision support

### Trade-offs Accepted
- Lower maximum throughput than Kafka (sufficient for our use case)
- Smaller ecosystem than Kafka (fewer connectors, but we control all producers/consumers)
- Less mature multi-datacenter replication (acceptable for MVP scope)

---

## Policy Engine: Open Policy Agent (OPA)

### Decision
Use OPA for policy enforcement at every stage of the pipeline.

### Rationale

**Separation of Concerns:**
- Policy logic is externalized from application code
- Rules can be updated without redeploying agents
- Security team can audit policies independently of code

**Declarative Policy Language:**
- Rego provides a purpose-built language for authorization decisions
- Policies are readable and auditable
- Built-in support for complex conditions, data lookups, and decision explanations

**Why OPA specifically:**
- Industry standard for cloud-native policy enforcement
- Supports bundle-based deployment matching our multi-policy structure
- Rich decision response format supports deny reasons, warnings, and metadata
- HTTP API integrates cleanly with Go services

**Policy Coverage:**
1. **Origin Attestation** - Validates message sources match expected patterns
2. **Data Handling** - Controls access based on classification levels
3. **Proposal Validation** - Ensures proposals meet requirements before human review
4. **Effect Release** - Enforces human approval chain for action execution

### Trade-offs Accepted
- Additional network hop for policy decisions (mitigated by local caching)
- Rego learning curve for policy authors
- Bundle synchronization complexity in multi-node deployments

---

## Agent Implementation: Go

### Decision
Implement all agents in Go.

### Rationale

**Performance:**
- Compiled language with minimal runtime overhead
- Efficient memory management for long-running services
- Native concurrency primitives (goroutines, channels) match event-driven architecture

**Operational Benefits:**
- Single static binary deployment (no runtime dependencies)
- Fast startup time (critical for container orchestration)
- Low memory footprint compared to JVM-based alternatives

**Ecosystem:**
- Excellent NATS client library (nats.go)
- Strong OpenTelemetry support
- First-class PostgreSQL driver (pgx)
- Mature HTTP routing (chi)

**Why not Rust:**
- Steeper learning curve for team velocity
- Go's garbage collection is acceptable for our latency requirements
- Better library ecosystem for the specific integrations needed

**Why not Java/Kotlin:**
- JVM startup time impacts container scaling
- Higher memory baseline for each agent instance
- Go better matches cloud-native deployment patterns

### Trade-offs Accepted
- Less expressive than Rust for some patterns
- Manual error handling (verbose but explicit)
- No generics until Go 1.18 (now resolved, but some patterns remain)

---

## Persistence: PostgreSQL

### Decision
Use PostgreSQL as the primary database for state persistence.

### Rationale

**Reliability:**
- Battle-tested durability guarantees
- ACID transactions for multi-table updates
- Strong consistency model matches our audit requirements

**Features Used:**
- JSONB columns for flexible schema evolution (sources, metadata, policy_result)
- PostgreSQL-specific types (UUID, TIMESTAMPTZ, ENUMs)
- Views for denormalized query patterns (pending_proposals_queue)
- Triggers for automatic timestamp updates

**Why not a NoSQL alternative:**
- Strong relationships between entities (tracks -> proposals -> decisions -> effects)
- Need for transactional updates across tables
- Complex queries for audit trail (JOIN-heavy)
- JSONB provides document flexibility where needed

**Why not Event Sourcing with only NATS:**
- Need materialized views for efficient queries
- Audit compliance requires durable, queryable records
- Historical analysis benefits from SQL capabilities

### Trade-offs Accepted
- Schema migrations required for evolution
- Single point of failure (mitigated by replication in production)
- Write latency higher than pure event store

---

## Message Envelope Design

### Decision
All messages include a standardized envelope with identity, routing, timing, security, and tracing fields.

### Structure
```go
type Envelope struct {
    MessageID     string    // UUIDv7 for time-ordering
    CorrelationID string    // Chain tracking across agents
    CausationID   string    // Parent message reference
    Source        string    // Agent ID
    SourceType    string    // Agent type
    Timestamp     time.Time // Creation time
    Signature     string    // HMAC-SHA256
    PolicyVersion string    // OPA bundle version
    TraceID       string    // OpenTelemetry trace
    SpanID        string    // OpenTelemetry span
}
```

### Rationale

**Traceability:**
- CorrelationID links all messages from a single detection through to effect execution
- CausationID enables causal ordering and event sourcing replay
- TraceID/SpanID integrate with distributed tracing

**Security:**
- Signature field supports message integrity verification
- Source/SourceType validated by OPA origin attestation policy
- PolicyVersion enables replay with historical policy context

**Operability:**
- MessageID with time-ordered UUIDs supports debugging and log correlation
- Timestamp captures authoritative creation time
- Standardized structure simplifies cross-agent tooling

### Trade-offs Accepted
- Message overhead (~200-300 bytes per message)
- Signature computation adds latency (currently HMAC, not asymmetric)
- PolicyVersion requires bundle version tracking

---

## Idempotency Strategy

### Decision
Implement idempotency at the effect execution layer using database-enforced unique keys.

### Implementation
```sql
CREATE TABLE effects (
    ...
    idempotent_key VARCHAR(128) UNIQUE NOT NULL,
    ...
);
```

The idempotent key is computed as:
```
SHA256(decision_id + proposal_id + action_type)
```

### Rationale

**Why Database-Enforced:**
- Unique constraint guarantees exactly-once semantics even with race conditions
- Survives process restarts (unlike in-memory deduplication)
- Audit trail shows both original execution and duplicate attempts

**Why at Effect Layer:**
- Effects are the only stage with real-world consequences
- Upstream stages can safely be replayed (idempotent transformations)
- Concentrates complexity where it matters most

**Alternative Considered: NATS Deduplication**
- NATS JetStream has built-in message deduplication
- Rejected because effect execution confirmation must survive beyond the message dedup window
- Database provides permanent record of execution status

### Trade-offs Accepted
- Additional database write in the critical path
- Key collision theoretical risk (SHA256 collision - practically zero)
- Requires cleanup of stale idempotency records (TTL-based)

---

## Human-in-the-Loop Enforcement

### Decision
All action proposals require explicit human approval with no automatic bypass.

### Implementation

**OPA Policy (effects/release.rego):**
```rego
# Human approval is ALWAYS required (safety constraint)
require_human := true

valid_approval_chain if {
    input.decision.approved == true
    input.decision.approved_by != ""
    input.decision.approved_by != "system"  # Explicit block
    input.decision.proposal_id == input.proposal.proposal_id
}
```

**Database Schema:**
```sql
approved_by VARCHAR(128) NOT NULL  -- Cannot be empty
```

### Rationale

**Safety:**
- Prevents runaway automation in a command and control context
- Matches real-world ROE (Rules of Engagement) requirements
- Demonstrates responsible AI integration patterns

**Auditability:**
- Every effect traces back to an identified human decision-maker
- Denial decisions are equally captured for analysis
- Supports after-action review and policy refinement

**Design Philosophy:**
- System proposes, human disposes
- AI/ML can inform priority and recommendations
- Final authority remains with trained operators

### Trade-offs Accepted
- Latency in the decision loop (intentional)
- Cannot demonstrate fully autonomous operation
- Operator workload in high-volume scenarios

---

## Proposal De-duplication Strategy

### Decision
Consolidate multiple sensor hits on the same track into a single pending proposal using database-enforced unique constraints.

### Implementation
```sql
-- Partial unique index ensures one pending proposal per track
CREATE UNIQUE INDEX idx_proposals_track_pending_unique
  ON proposals(track_id)
  WHERE status = 'pending';

-- Additional columns for hit tracking
ALTER TABLE proposals ADD COLUMN hit_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE proposals ADD COLUMN last_hit_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
```

**UPSERT Logic (Authorizer Agent)**:
```go
// If proposal exists for this track, UPDATE it
// Otherwise, INSERT new one
ON CONFLICT (track_id) WHERE status = 'pending'
DO UPDATE SET
  hit_count = proposals.hit_count + 1,
  last_hit_at = NOW(),
  priority = GREATEST(proposals.priority, EXCLUDED.priority)
```

### Rationale

**Problem Solved:**
- Without de-duplication, a single hostile aircraft generates 100+ proposals in minutes
- Operator UI becomes cluttered with redundant entries
- Same track appears multiple times in the approval queue

**Why Database-Enforced:**
- Partial unique index guarantees consistency under concurrent writes
- No application-level locking or distributed coordination required
- Survives agent restarts (stateless agents)

**Why Track Hit Count:**
- Provides operational intelligence (track persistence indicator)
- Higher hit counts correlate with higher confidence
- UI can prioritize frequently-detected threats

### Trade-offs Accepted
- Additional database write per duplicate detection (UPSERT vs INSERT)
- Slightly more complex query logic in Authorizer
- Hit count not meaningful for single-detection proposals

---

## WebSocket Hub Architecture

### Decision
Implement a centralized pub/sub WebSocket hub that bridges NATS events to connected browser clients.

### Implementation
```go
type WebSocketHub struct {
    clients    map[*Client]bool
    broadcast  chan *Message
    register   chan *Client
    unregister chan *Client
    subscriptions map[*Client]map[string]bool
}
```

**Message Flow:**
```
NATS JetStream → Hub.Subscribe() → Hub.broadcast → Client.send
```

### Rationale

**Why Centralized Hub:**
- Decouples WebSocket clients from direct NATS subscriptions
- Allows filtering and transformation before client delivery
- Single point for connection management and cleanup

**Why Not Direct NATS WebSocket:**
- NATS WebSocket requires clients to understand NATS protocol
- Browser clients expect JSON messages, not NATS wire format
- Hub allows message aggregation and rate limiting

**Subscription Model:**
- Clients can subscribe to specific message types (track.update, proposal.new, etc.)
- If no subscriptions set, client receives ALL messages (default)
- Supports dynamic subscribe/unsubscribe at runtime

### Trade-offs Accepted
- Additional memory for Hub state (client tracking)
- Single broadcast channel could become bottleneck at scale
- Not horizontally scalable without shared state (Redis pub/sub)

---

## Frontend State Management Pattern

### Decision
Combine Zustand (UI state), TanStack React Query (server state), and WebSocket hook (real-time updates) for frontend architecture.

### Implementation
```
+-------------+     +---------------+     +-------------+
|   Zustand   |     | React Query   |     | WebSocket   |
| (UI State)  |     | (Server State)|     | (Real-time) |
+-------------+     +---------------+     +-------------+
       |                   |                    |
       v                   v                    v
   Modal open?         Tracks[]            Invalidate
   Selected tab        Proposals[]         Query Cache
   Filter state        Decisions[]
```

**Cache Invalidation Pattern:**
```typescript
// WebSocket receives new proposal
onMessage('proposal.new', () => {
  queryClient.invalidateQueries(['proposals'])
})
```

### Rationale

**Why Three Layers:**
- Zustand: Lightweight, no boilerplate, perfect for transient UI state
- React Query: Handles caching, retry, deduplication automatically
- WebSocket: Real-time updates without polling

**Why Invalidation over Direct Updates:**
- Simpler than manually merging WebSocket data with cache
- Query refetch ensures data consistency
- Works seamlessly with optimistic updates

### Trade-offs Accepted
- Additional network request on invalidation (vs direct cache update)
- Three libraries to maintain
- Learning curve for developers unfamiliar with React Query

---

## Trade-off Summary

### Performance vs Durability
- **Choice:** Durability
- JetStream file storage over memory-only
- Database writes in critical path
- Acceptable latency for decision support (not real-time control)

### Simplicity vs Flexibility
- **Choice:** Balanced
- Go over Rust (simpler, sufficient performance)
- PostgreSQL over event-only store (simpler queries)
- OPA over embedded rules (more flexible policy management)

### Autonomy vs Safety
- **Choice:** Safety
- Mandatory human approval
- No automatic effect execution
- Policy-enforced at multiple layers

---

## Future Roadmap

### Recently Completed
- [x] **Proposal de-duplication** - Same-entity sensor hits consolidated into single proposals
- [x] **WebSocket hub architecture** - Real-time updates to browser clients
- [x] **Sensor runtime configuration** - HTTP API for dynamic sensor tuning
- [x] **Consumer resilience** - Automatic NATS consumer recreation on failure
- [x] **Metrics dashboard** - Real-time pipeline metrics and latency tracking
- [x] **Audit trail enhancements** - Reason field and improved search

### Short Term
- [ ] Add asymmetric signature verification (replace HMAC with Ed25519)
- [ ] Implement proposal prioritization ML model
- [ ] Add Grafana dashboards for operational metrics
- [ ] WebSocket message compression

### Medium Term
- [ ] Multi-datacenter NATS clustering
- [ ] PostgreSQL read replicas for query scaling
- [ ] Role-based access control in API gateway
- [ ] Batch proposal review interface
- [ ] mTLS for production NATS communication

### Long Term
- [ ] Edge deployment topology (disconnected operations)
- [ ] Federated learning for classifier improvement
- [ ] Integration with Link 16 / VMF message formats
- [ ] Formal verification of OPA policies

---

## References

- [NATS JetStream Documentation](https://docs.nats.io/nats-concepts/jetstream)
- [OPA Policy Language](https://www.openpolicyagent.org/docs/latest/policy-language/)
- [PostgreSQL JSONB](https://www.postgresql.org/docs/current/datatype-json.html)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/instrumentation/go/)
