# CJADC2

Event-driven command and control decision support platform using NATS JetStream.

## Quick Start

```bash
docker compose up --build
```

## Services

| Service    | URL                    |
|------------|------------------------|
| UI         | http://localhost:3000  |
| API        | http://localhost:8080  |
| NATS       | http://localhost:8222  |
| Prometheus | http://localhost:9090  |
| Jaeger     | http://localhost:16686 |
| OPA        | http://localhost:8181  |

## Architecture

```
Sensor --> Classifier --> Correlator --> Planner --> Authorizer --> Effector
   |           |              |            |             |             |
   v           v              v            v             v             v
+---------------------------------------------------------------------+
|                      NATS JetStream                                 |
+---------------------------------------------------------------------+
                               |
              +----------------+----------------+
              |                |                |
              v                v                v
         PostgreSQL          OPA           API Gateway
         (persistence)    (policies)       (REST + WS)
                                               |
                                               v
                                           React UI
```

**Agents:**
- Sensor: Generates synthetic detections
- Classifier: Adds classification and track type
- Correlator: Fuses tracks, assigns threat levels
- Planner: Generates action proposals
- Authorizer: Human-in-the-loop approval
- Effector: Executes approved actions

## Directory Structure

```
cmd/agents/     # Agent implementations
cmd/api-gateway # REST API server
pkg/            # Shared libraries
policies/       # OPA Rego policies
migrations/     # PostgreSQL schema
ui/             # React frontend
```

## License

Proprietary. All rights reserved.
