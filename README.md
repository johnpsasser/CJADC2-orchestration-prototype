# CJADC2

**Combined Joint All-Domain Command and Control** - An event-driven orchestration platform for accelerating complex mission decision chains with human-in-the-loop approval gates.

![System Architecture](docs/cjadc2-system-architecture.png)

## Overview

CJADC2 demonstrates an end-to-end pipeline from sensor detection to human-approved action execution. The platform uses event streaming with exactly-once delivery semantics, policy-as-code enforcement, and comprehensive audit trails.

**Core Pipeline:** `Sensor → Classify → Correlate → Plan → Request Approval → Execute`

## Quick Start

```bash
# Start the complete stack
docker compose up --build

# Or use Make targets
make up-build    # Build and start
make status      # Check health
make logs        # View logs
```

## Services

| Service    | URL                      | Description                          |
|------------|--------------------------|--------------------------------------|
| UI         | http://localhost:3000    | React dashboard                      |
| API        | http://localhost:8080    | REST API + WebSocket                 |
| NATS       | http://localhost:8222    | Message broker monitoring            |
| Prometheus | http://localhost:9090    | Metrics collection                   |
| Jaeger     | http://localhost:16686   | Distributed tracing                  |
| OPA        | http://localhost:8181    | Policy engine                        |
| PostgreSQL | localhost:5432           | Database (cjadc2:devpassword)        |

## Architecture

![Agent Pipeline](docs/images/agent-pipeline.png)

### Agent Pipeline

```
Sensor (detect.>)
    ↓
Classifier (track.classified.{classification})
    ↓
Correlator (track.correlated.{threat_level})
    ↓
Planner (proposal.pending.{priority})
    ↓
Authorizer (decision.{approved|denied}.{action_type})
    ↓
Effector (effect.{status}.{action_type})
```

| Agent      | Responsibility                                              |
|------------|-------------------------------------------------------------|
| Sensor     | Generates synthetic detection events with position/velocity |
| Classifier | Adds classification (friendly/hostile/neutral/unknown)      |
| Correlator | Fuses duplicate tracks, assigns threat levels               |
| Planner    | Generates action proposals based on OPA policy evaluation   |
| Authorizer | Handles human-in-the-loop approval via UI/API               |
| Effector   | Executes approved actions with idempotency guarantees       |

### Technology Stack

- **Message Bus:** NATS JetStream 2.10 - Event streaming with exactly-once delivery
- **Database:** PostgreSQL 16 - State persistence and ACID-compliant audit trails
- **Policy Engine:** OPA 0.60 - Rego-based policy enforcement at every stage
- **Backend:** Go 1.22 - All agents and API gateway
- **Frontend:** React 18 + TypeScript + Zustand + TanStack Query
- **Observability:** Prometheus metrics, Jaeger tracing, Zerolog structured logging

![Data Flow](docs/images/data-flow.png)

## Documentation

Detailed documentation is available in the [`docs/`](docs/) directory:

| Document | Description |
|----------|-------------|
| [Technical PRD](docs/Technical%20PRD%20-%20CJADC2%20Orchestration%20Platform.pdf) | Product requirements, goals, KPIs, threat model |
| [System Architecture](docs/System%20Architecture%20-%20CJADC2%20Orchestration%20Platform.pdf) | Component design, message flows, deployment topology |
| [Architecture Decision Records](docs/Architecture%20Decision%20Records%20-%20CJADC2%20Orchestration%20Platform.pdf) | Technical decisions and rationale |
| [API Reference](docs/API%20Reference%20-%20CJADC2%20Orchestration%20Platform.pdf) | REST endpoint documentation |

### Architecture Diagrams

- [`docs/images/system-architecture.png`](docs/images/system-architecture.png) - High-level system overview
- [`docs/images/agent-pipeline.png`](docs/images/agent-pipeline.png) - Agent processing flow
- [`docs/images/api-architecture.png`](docs/images/api-architecture.png) - API layer design
- [`docs/images/data-flow.png`](docs/images/data-flow.png) - Message flow through system
- [`docs/images/broker-comparison.png`](docs/images/broker-comparison.png) - NATS vs Kafka vs RabbitMQ
- [`docs/images/trade-off-summary.png`](docs/images/trade-off-summary.png) - Technology trade-off matrix

## Directory Structure

```
cmd/
├── agents/
│   ├── sensor/         # Synthetic detection generator
│   ├── classifier/     # Classification enrichment
│   ├── correlator/     # Track fusion & threat assessment
│   ├── planner/        # Proposal generation with OPA
│   ├── authorizer/     # Human approval workflow
│   └── effector/       # Effect execution
└── api-gateway/        # REST API + WebSocket server

pkg/
├── agent/              # BaseAgent framework (NATS, metrics, logging)
├── handler/            # HTTP handlers
├── messages/           # Message types and Envelope
├── postgres/           # Database queries
└── observability/      # Prometheus + Jaeger integration

policies/bundles/cjadc2/  # OPA Rego policies
migrations/               # PostgreSQL schema
ui/src/                   # React dashboard
configs/                  # NATS, Prometheus, Grafana configs
```

## Development

### Local Development

```bash
# Start infrastructure only
make infra

# Run agents locally
go run ./cmd/agents/sensor
go run ./cmd/agents/classifier
go run ./cmd/api-gateway

# Run UI with hot reload
cd ui && npm run dev
```

### Testing

```bash
make test            # Run all Go tests
make test-cover      # Generate coverage report
make test-race       # Run with race detector
cd ui && npm run lint        # Frontend linting
cd ui && npm run type-check  # TypeScript validation
```

### Debugging

```bash
make health          # Check all service health
make db-cli          # PostgreSQL interactive shell
make nats-cli        # NATS monitoring CLI
docker compose logs sensor-sim -f  # Follow specific agent
```

## API Examples

```bash
# List active tracks
curl -s localhost:8080/api/v1/tracks | jq '.tracks'

# Get pending proposals
curl -s localhost:8080/api/v1/proposals | jq '.proposals'

# Submit a decision
curl -X POST localhost:8080/api/v1/decisions \
  -H "Content-Type: application/json" \
  -d '{"proposal_id":"<id>","approved":true,"reason":"Authorized engagement"}'

# View audit trail
curl -s localhost:8080/api/v1/audit | jq '.entries'
```

## Configuration

Environment variables with defaults:

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | nats://localhost:4222 | NATS connection |
| `POSTGRES_URL` | postgres://cjadc2:devpassword@localhost:5432/cjadc2 | Database |
| `OPA_URL` | http://localhost:8181 | Policy engine |
| `EMISSION_INTERVAL` | 500ms | Sensor detection rate |
| `CORRELATION_WINDOW` | 10s | Track fusion window |
| `TRACK_COUNT` | 10 | Concurrent simulated tracks |

## Key Design Decisions

See [Architecture Decision Records](docs/Architecture%20Decision%20Records%20-%20CJADC2%20Orchestration%20Platform.pdf) for full rationale.

- **NATS JetStream over Kafka** - Single binary, sub-millisecond latency, native stream semantics
- **OPA for Policy** - Separation of concerns, declarative Rego, bundle-based updates
- **Go for Agents** - Compiled performance, goroutine concurrency, excellent NATS ecosystem
- **PostgreSQL over NoSQL** - ACID transactions for audit integrity, complex query support

## License

Proprietary. All rights reserved.
