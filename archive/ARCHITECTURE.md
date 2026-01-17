# Architecture Documentation

This document provides a comprehensive technical overview of the CJADC2 platform architecture, including component descriptions, data flows, and deployment topology.

## System Overview

The CJADC2 platform implements an event-driven architecture for command and control decision support. The system processes sensor detections through a pipeline of specialized agents, each performing a distinct function in the intelligence-to-action workflow.

### Design Principles

1. **Event Sourcing**: All state changes are captured as immutable events
2. **Separation of Concerns**: Each agent has a single responsibility
3. **Policy as Code**: Authorization logic externalized to OPA
4. **Human-in-the-Loop**: All consequential actions require human approval
5. **Observability First**: Full tracing and metrics from day one

## Component Architecture

```
+------------------------------------------------------------------+
|                        External Interface                         |
|  +------------------+  +------------------+  +------------------+ |
|  |    React UI      |  |   REST API       |  |   WebSocket      | |
|  |   (Dashboard)    |  |   (Gateway)      |  |   (Real-time)    | |
|  +--------+---------+  +--------+---------+  +--------+---------+ |
+-----------|--------------------|--------------------|-------------+
            |                    |                    |
            v                    v                    v
+------------------------------------------------------------------+
|                         API Gateway                               |
|  +------------------+  +------------------+  +------------------+ |
|  | Track Handler    |  | Proposal Handler |  | Decision Handler | |
|  +------------------+  +------------------+  +------------------+ |
+------------------------------------------------------------------+
            |                    |                    |
            v                    v                    v
+------------------------------------------------------------------+
|                     NATS JetStream                                |
|  +------------+  +------------+  +------------+  +------------+  |
|  | DETECTIONS |  |   TRACKS   |  | PROPOSALS  |  | DECISIONS  |  |
|  |   Stream   |  |   Stream   |  |   Stream   |  |   Stream   |  |
|  +------------+  +------------+  +------------+  +------------+  |
+------------------------------------------------------------------+
            |                    |                    |
            v                    v                    v
+------------------------------------------------------------------+
|                         Agent Layer                               |
|  +--------+  +----------+  +----------+  +-------+  +---------+  |
|  | Sensor |->| Classify |->| Correlate|->|Planner|->|Authorize|  |
|  +--------+  +----------+  +----------+  +-------+  +---------+  |
|                                                           |      |
|  +--------+                                               |      |
|  |Effector|<----------------------------------------------+      |
|  +--------+                                                      |
+------------------------------------------------------------------+
            |                    |                    |
            v                    v                    v
+------------------------------------------------------------------+
|                      Support Services                             |
|  +------------------+  +------------------+  +------------------+ |
|  |       OPA        |  |   PostgreSQL     |  |     Jaeger       | |
|  | (Policy Engine)  |  |   (Persistence)  |  |   (Tracing)      | |
|  +------------------+  +------------------+  +------------------+ |
+------------------------------------------------------------------+
```

## Agent Descriptions

### Sensor Agent

**Purpose**: Generate synthetic sensor detection events

**Responsibilities**:
- Simulate radar, EO, and SIGINT sensor systems
- Generate realistic track positions with motion models
- Emit detection events at configurable intervals
- Attach origin metadata for policy validation
- Provide HTTP API for runtime configuration

**Configuration**:
| Variable | Default | Description |
|----------|---------|-------------|
| EMISSION_INTERVAL | 500ms | Time between detections |
| TRACK_COUNT | 10 | Number of concurrent tracks |
| SENSOR_TYPE | radar | Simulated sensor type |
| TRACK_TYPE_WEIGHTS | equal | Distribution of track types (aircraft, vessel, ground, missile, unknown) |
| CLASSIFICATION_WEIGHTS | equal | Distribution of classifications (friendly, hostile, neutral, unknown) |

**HTTP Control API** (Port 9090):
- `GET /api/v1/config` - Get current sensor configuration
- `PATCH /api/v1/config` - Update emission interval, track count, or weights at runtime
- `POST /api/v1/config/reset` - Reset to default configuration
- `PATCH /api/v1/config` with `clear_streams: true` - Purge all NATS streams and consumers

**Output**: `detect.{sensor_id}.{sensor_type}`

### Classifier Agent

**Purpose**: Enrich detections with classification and type information

**Responsibilities**:
- Consume raw detection events
- Apply classification rules (friendly/hostile/unknown/neutral)
- Determine track type (aircraft/vessel/ground/missile/unknown)
- Propagate correlation context
- Validate data handling permissions via OPA

**Special Classification Rules**:
- Missile tracks use biased classification weights (90% hostile, 10% unknown)
- Track ID prefixes influence classification: `F-TRK-*` (friendly), `H-TRK-*` (hostile), `N-TRK-*` (neutral), `U-TRK-*` (unknown)

**Input**: `detect.>` (DETECTIONS stream)
**Output**: `track.classified.{classification}`

### Correlator Agent

**Purpose**: Fuse multiple tracks and assess threat levels

**Responsibilities**:
- Maintain sliding window of classified tracks
- Merge duplicate tracks from multiple sensors
- Calculate fused confidence scores
- Assign threat levels based on behavior analysis
- Output correlated tracks for planning

**Configuration**:
| Variable | Default | Description |
|----------|---------|-------------|
| CORRELATION_WINDOW | 10s | Time window for track fusion |

**Input**: `track.classified.>` (TRACKS stream)
**Output**: `track.correlated.{threat_level}`

### Planner Agent

**Purpose**: Generate action proposals for operator review

**Responsibilities**:
- Analyze correlated tracks for actionable intelligence
- Generate proposals with rationale and priority
- Validate proposals against OPA policy rules
- Set expiration times for time-sensitive actions (default: 5 minutes)
- Handle policy warnings and adjustments
- Route proposals based on human-in-the-loop requirements

**Human-in-the-Loop Logic**:
| Action Type | HITL Required | Condition |
|-------------|---------------|-----------|
| engage | Always | Kinetic actions always require human approval |
| intercept | Always | Kinetic actions always require human approval |
| identify | Conditional | Required when priority >= 6 |
| track | Never | Passive observation auto-approved |
| monitor | Never | Passive observation auto-approved |
| ignore | Never | Passive action auto-approved |

**Input**: `track.correlated.>` (TRACKS stream)
**Output**: `proposal.pending.{priority}`

### Authorizer Agent

**Purpose**: Manage human approval workflow

**Responsibilities**:
- Present proposals to operators via API
- Capture approval/denial decisions with operator identity
- Enforce decision timeout (proposal expiration)
- Persist decisions to database for audit
- Publish approved decisions for effect execution
- **De-duplicate proposals for same-entity sensor hits**

**Proposal De-duplication**:
When multiple sensor detections target the same track, proposals are consolidated:
- Database-enforced unique constraint on `(track_id)` WHERE `status = 'pending'`
- New detections UPSERT existing proposals instead of creating duplicates
- `hit_count` tracks number of sensor hits consolidated into the proposal
- `last_hit_at` records the most recent detection timestamp
- Priority is updated if the new detection has higher priority

**Input**: `proposal.>` (PROPOSALS stream)
**Output**: `decision.{approved|denied}.{action_type}`

### Effector Agent

**Purpose**: Execute approved actions with safety guarantees

**Responsibilities**:
- Consume only approved decisions
- Validate approval chain via OPA policy
- Execute action with idempotency protection
- Log effect results for audit compliance
- Publish execution status

**Input**: `decision.approved.>` (DECISIONS stream)
**Output**: `effect.{status}.{action_type}`

## Data Flow

### Detection to Effect Pipeline

```
1. Sensor generates detection
   +------------------+
   | Detection        |
   | - track_id       |
   | - position       |
   | - velocity       |
   | - confidence     |
   | - sensor_type    |
   +------------------+
           |
           v
2. Classifier enriches track
   +------------------+
   | Track            |
   | - track_id       |
   | - classification |
   | - type           |
   | - position       |
   | - confidence     |
   +------------------+
           |
           v
3. Correlator fuses data
   +------------------+
   | CorrelatedTrack  |
   | - track_id       |
   | - merged_from[]  |
   | - threat_level   |
   | - fused_conf     |
   +------------------+
           |
           v
4. Planner creates proposal
   +------------------+
   | ActionProposal   |
   | - proposal_id    |
   | - action_type    |
   | - priority       |
   | - rationale      |
   | - track          |
   +------------------+
           |
           v
5. Human approves/denies
   +------------------+
   | Decision         |
   | - decision_id    |
   | - approved       |
   | - approved_by    |
   | - reason         |
   +------------------+
           |
           v
6. Effector executes
   +------------------+
   | EffectLog        |
   | - effect_id      |
   | - status         |
   | - result         |
   | - idempotent_key |
   +------------------+
```

### Message Correlation

Every message carries an envelope with correlation fields:

```
CorrelationID: Links all messages from a single originating detection
CausationID:   References the immediate parent message
MessageID:     Unique identifier for this specific message

Example chain:
  Detection[msg-001, corr-ABC, cause-nil]
       |
       v
  Track[msg-002, corr-ABC, cause-msg-001]
       |
       v
  Correlated[msg-003, corr-ABC, cause-msg-002]
       |
       v
  Proposal[msg-004, corr-ABC, cause-msg-003]
       |
       v
  Decision[msg-005, corr-ABC, cause-msg-004]
       |
       v
  Effect[msg-006, corr-ABC, cause-msg-005]
```

## NATS Stream Design

### Stream Definitions

| Stream | Subjects | Retention | Max Age | Purpose |
|--------|----------|-----------|---------|---------|
| DETECTIONS | detect.> | Limits | 24h | Raw sensor data |
| TRACKS | track.> | Limits | 72h | Classified/correlated tracks |
| PROPOSALS | proposal.> | WorkQueue | 1h | Pending approvals |
| DECISIONS | decision.> | Limits | 7d | Human decisions |
| EFFECTS | effect.> | Limits | 30d | Execution records |

### Subject Hierarchy

```
detect.
  +-- {sensor_id}.
        +-- radar
        +-- eo
        +-- sigint

track.
  +-- classified.
  |     +-- friendly
  |     +-- hostile
  |     +-- unknown
  |     +-- neutral
  +-- correlated.
        +-- low
        +-- medium
        +-- high
        +-- critical

proposal.
  +-- pending.
        +-- normal
        +-- medium
        +-- high

decision.
  +-- approved.
  |     +-- engage
  |     +-- track
  |     +-- identify
  |     +-- ...
  +-- denied.
        +-- engage
        +-- track
        +-- ...

effect.
  +-- executed.
  +-- failed.
  +-- simulated.
```

### Consumer Configuration

| Consumer | Stream | Filter | Ack Wait | Max Deliver |
|----------|--------|--------|----------|-------------|
| classifier | DETECTIONS | detect.> | 30s | 3 |
| correlator | TRACKS | track.classified.> | 30s | 3 |
| planner | TRACKS | track.correlated.> | 30s | 3 |
| authorizer | PROPOSALS | proposal.> | 300s | 1 |
| effector | DECISIONS | decision.approved.> | 60s | 5 |

Note: Authorizer has MaxDeliver=1 because human decisions should not be retried.

## OPA Policy Structure

### Bundle Layout

```
policies/bundles/cjadc2/
+-- origin/
|   +-- attestation.rego    # Source validation
+-- data_handling/
|   +-- classification.rego # Data access control
+-- proposals/
|   +-- rules.rego         # Proposal validation
+-- effects/
    +-- release.rego       # Effect authorization
```

### Policy Evaluation Points

```
Agent              Policy Path              Purpose
------             -----------              -------
All agents    -->  cjadc2/origin       --> Validate message source
Classifier    -->  cjadc2/data_handling--> Check data clearance
Planner       -->  cjadc2/proposals    --> Validate proposal rules
Effector      -->  cjadc2/effects      --> Verify approval chain
```

### Decision Response Format

```json
{
  "allowed": true,
  "reasons": [],
  "warnings": ["Priority may be too low for threat level"],
  "metadata": {
    "source": "planner-001",
    "action_type": "track",
    "policy_version": "v1.2.0"
  }
}
```

## Database Schema

### Entity Relationship Diagram

```
+------------------+       +------------------+       +------------------+
|     tracks       |       |    proposals     |       |    decisions     |
+------------------+       +------------------+       +------------------+
| PK track_id      |<------| FK track_id      |<------| FK proposal_id   |
|    external_id   |       | PK proposal_id   |       | PK decision_id   |
|    classification|       |    action_type   |       |    approved      |
|    type          |       |    priority      |       |    approved_by   |
|    confidence    |       |    rationale     |       |    reason        |
|    position_*    |       |    status        |       +--------+---------+
|    velocity_*    |       |    expires_at    |                |
|    threat_level  |       |    hit_count     |                |
|    state         |       |    last_hit_at   |                |
|    detection_cnt |       +--------+---------+                |
+--------+---------+                |                          |
         |                          |                          v
         |                          |               +------------------+
         v                          |               |     effects      |
+------------------+                |               +------------------+
|   detections     |                +-------------->| FK decision_id   |
+------------------+                                | FK proposal_id   |
| PK detection_id  |                                | FK track_id      |
| FK track_id      |                                | PK effect_id     |
|    sensor_id     |                                |    status        |
|    position_*    |                                |    idempotent_key|
|    raw_data      |                                +------------------+
+------------------+
                                    +------------------+
                                    |    audit_log     |
                                    +------------------+
                                    |    entity_type   |
                                    |    entity_id     |
                                    |    action        |
                                    |    actor_id      |
                                    |    old_value     |
                                    |    new_value     |
                                    +------------------+
```

### Key Indexes

- `idx_tracks_state` - Filter active tracks
- `idx_tracks_threat_level` - Priority sorting
- `idx_proposals_status` - Pending queue queries
- `idx_proposals_track_pending_unique` - **Partial unique index** on `(track_id)` WHERE `status = 'pending'` for proposal de-duplication
- `idx_effects_idempotent_key` - Deduplication lookups
- `idx_audit_log_correlation_id` - Chain reconstruction

### Materialized Views

- `pending_proposals_queue` - Optimized proposal dashboard
- `active_tracks_view` - Current operational picture
- `decision_audit_trail` - Full decision chain with effects

## Observability Stack

### Metrics (Prometheus)

**Agent Metrics**:
```
agent_messages_total{status, message_type}     # Message throughput
agent_processing_latency_seconds{message_type} # Processing time histogram
agent_errors_total{error_type}                 # Error counts
```

**Infrastructure Metrics**:
- NATS: connections, messages, bytes, jetstream stats
- PostgreSQL: connections, query duration, cache hits
- OPA: decision latency, policy evaluation counts

### Tracing (Jaeger)

All messages include OpenTelemetry context:
- TraceID: Spans entire detection-to-effect flow
- SpanID: Individual agent processing
- Parent propagation through message envelope

**Trace Example**:
```
Trace: abc123
+-- Sensor.Emit (12ms)
    +-- Classifier.Process (8ms)
        +-- OPA.CheckOrigin (2ms)
        +-- OPA.CheckDataHandling (2ms)
        +-- Correlator.Process (15ms)
            +-- Planner.Generate (25ms)
                +-- OPA.CheckProposal (3ms)
                +-- [Human Decision - 45s]
                    +-- Effector.Execute (18ms)
                        +-- OPA.CheckEffectRelease (2ms)
```

### Logging (Zerolog)

Structured JSON logging with consistent fields:
```json
{
  "level": "info",
  "time": "2024-01-15T10:30:00Z",
  "agent_id": "classifier-001",
  "agent_type": "classifier",
  "message_id": "msg-uuid",
  "correlation_id": "corr-uuid",
  "message": "Processed detection"
}
```

## Deployment Topology

### Development (Docker Compose)

```
+---------------------------------------------------------------+
|                    Docker Network: cjadc2                     |
|                                                               |
|  +-------+  +-------+  +-------+  +-------+  +-------+       |
|  | nats  |  |  opa  |  |  pg   |  |jaeger |  |  ui   |       |
|  | :4222 |  | :8181 |  | :5432 |  |:16686 |  | :3000 |       |
|  +-------+  +-------+  +-------+  +-------+  +-------+       |
|                                                               |
|  +--------+  +----------+  +----------+  +--------+          |
|  | sensor |  |classifier|  |correlator|  |planner |          |
|  +--------+  +----------+  +----------+  +--------+          |
|                                                               |
|  +----------+  +--------+  +-----------+                     |
|  |authorizer|  |effector|  |api-gateway|                     |
|  +----------+  +--------+  |   :8080   |                     |
|                            +-----------+                     |
+---------------------------------------------------------------+
```

### Production Considerations

**Edge Deployment**:
```
                    Cloud Region
+--------------------------------------------------+
|  +-------+  +-------+  +-------+                |
|  | NATS  |  |  OPA  |  |  PG   |                |
|  |Cluster|  |Cluster|  | Primary|               |
|  +---+---+  +-------+  +---+---+                |
|      |                     |                    |
+------|---------------------|--------------------+
       |                     |
       | (NATS Leaf/Bridge)  | (Replication)
       |                     |
+------|---------------------|--------------------+
|      v                     v                    |
|  +-------+            +-------+                 |
|  | NATS  |            |  PG   |                 |
|  | Leaf  |            |Replica|                 |
|  +-------+            +-------+                 |
|                                                 |
|  [Agents run locally with leaf NATS]           |
|  [OPA bundles synced periodically]             |
|                                                 |
|                    Edge Site                    |
+-------------------------------------------------+
```

**Scaling Considerations**:
- Agents: Horizontally scalable via consumer groups
- NATS: Clustered with JetStream replication
- PostgreSQL: Read replicas for query load
- OPA: Multiple instances with shared bundles
- API Gateway: Load balanced, stateless

## Security Architecture

### Network Segmentation

```
+-------------------+     +-------------------+     +-------------------+
|   Public Zone     |     |   Service Zone    |     |   Data Zone       |
|                   |     |                   |     |                   |
|  +-------------+  |     |  +-------------+  |     |  +-------------+  |
|  |     UI      |--|---->|  |  API GW     |--|---->|  | PostgreSQL  |  |
|  +-------------+  |     |  +-------------+  |     |  +-------------+  |
|                   |     |        |          |     |                   |
|                   |     |        v          |     |  +-------------+  |
|                   |     |  +-------------+  |     |  |    NATS     |  |
|                   |     |  |   Agents    |--|---->|  +-------------+  |
|                   |     |  +-------------+  |     |                   |
|                   |     |        |          |     |  +-------------+  |
|                   |     |        v          |     |  |    OPA      |  |
|                   |     |  +-------------+  |---->|  +-------------+  |
+-------------------+     +-------------------+     +-------------------+
```

### Authentication Flow

```
1. API Request
   +--------+
   | Client |---> [TLS] ---> API Gateway
   +--------+

2. Agent-to-NATS
   +-------+
   | Agent |---> [User/Pass] ---> NATS
   +-------+      (per-agent credentials)

3. Agent-to-OPA
   +-------+
   | Agent |---> [HTTP] ---> OPA
   +-------+      (internal network only)

4. Agent-to-PostgreSQL
   +-------+
   | Agent |---> [Conn String] ---> PostgreSQL
   +-------+      (service account)
```

### NATS Authorization Matrix

| Agent | Publish | Subscribe |
|-------|---------|-----------|
| sensor | detect.> | (none) |
| classifier | track.classified.> | detect.> |
| correlator | track.correlated.> | track.classified.> |
| planner | proposal.> | track.correlated.> |
| authorizer | decision.> | proposal.> |
| effector | effect.> | decision.approved.> |
| api | (all) | (all) |

## Performance Characteristics

### Latency Budget

| Stage | Target | Notes |
|-------|--------|-------|
| Sensor -> Detection | <10ms | Message creation and publish |
| Detection -> Track | <50ms | Classification + OPA check |
| Track -> Correlated | <100ms | Correlation window processing |
| Correlated -> Proposal | <50ms | Proposal generation + OPA |
| Human Decision | Variable | Not in latency budget |
| Decision -> Effect | <100ms | Execution + idempotency check |

**End-to-End (excluding human)**: <500ms target

### Throughput Targets

| Metric | Target | Bottleneck |
|--------|--------|------------|
| Detections/sec | 1,000 | Sensor emission |
| Tracks/sec | 500 | Classifier processing |
| Proposals/sec | 100 | Planner generation |
| Effects/sec | 50 | Database writes |

### Resource Requirements (per agent)

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| CPU | 0.1 cores | 0.5 cores |
| Memory | 64MB | 256MB |
| Network | 10Mbps | 100Mbps |

## Additional Configuration

### Environment Variables

All agents support these common environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| NATS_URL | nats://localhost:4222 | NATS JetStream connection URL |
| POSTGRES_URL | postgres://... | PostgreSQL connection string |
| OPA_URL | http://localhost:8181 | Open Policy Agent endpoint |
| AGENT_ID | auto-generated | Unique agent identifier |
| AGENT_TYPE | (required) | Agent type (sensor, classifier, etc.) |
| SIGNING_SECRET | (required) | HMAC-SHA256 signing key for message signatures |
| METRICS_ADDR | :9090 | HTTP metrics server bind address |
| OTEL_EXPORTER_OTLP_ENDPOINT | localhost:4317 | OpenTelemetry Jaeger endpoint |

## Consumer Resilience

Agents implement automatic consumer recreation to handle NATS consumer lifecycle events:

**Recovery Triggers**:
- "no responders" - NATS cluster temporarily unavailable
- "consumer not found" - Consumer was deleted
- "consumer deleted" - Consumer removed during rebalancing

**Recovery Behavior**:
1. Agent detects consumer error during message fetch
2. Logs warning and waits briefly (backoff)
3. Recreates consumer with original configuration
4. Resumes message processing

This pattern ensures agents survive:
- NATS cluster restarts
- Consumer rebalancing during scaling
- Manual consumer deletion during maintenance
