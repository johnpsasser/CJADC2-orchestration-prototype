# API Documentation

This document describes the REST API and WebSocket interfaces provided by the CJADC2 API Gateway.

## Base URL

```
http://localhost:8080
```

## Authentication

> Note: Authentication is not implemented in the MVP. All endpoints are currently open.

Future implementation will support:
- JWT Bearer tokens
- API key authentication
- Role-based access control

## REST API

### Health Check

#### GET /health

Check the health status of the API Gateway and its dependencies.

**Response**

```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "services": {
    "nats": "connected",
    "postgres": "connected",
    "opa": "connected"
  }
}
```

**Status Codes**

| Code | Description |
|------|-------------|
| 200 | All services healthy |
| 503 | One or more services unhealthy |

---

### Tracks

#### GET /api/v1/tracks

List all active tracks.

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| classification | string | - | Filter by: friendly, hostile, unknown, neutral |
| threat_level | string | - | Filter by threat: low, medium, high, critical, unknown |
| type | string | - | Filter by type: aircraft, vessel, ground, missile, unknown |
| limit | int | 100 | Maximum results to return |
| offset | int | 0 | Pagination offset |
| since | datetime | 60s ago | Only return tracks updated after this time (ISO 8601) |

> **Note:** By default, tracks are filtered to only return those updated within the last 60 seconds. Specify an explicit `since` parameter to override this behavior.

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/tracks?threat_level=high&limit=10"
```

**Response**

```json
{
  "tracks": [
    {
      "track_id": "550e8400-e29b-41d4-a716-446655440000",
      "external_track_id": "TRK-001",
      "classification": "hostile",
      "type": "aircraft",
      "confidence": 0.85,
      "position": {
        "lat": 34.0522,
        "lon": -118.2437,
        "alt": 10000
      },
      "velocity": {
        "speed": 250.5,
        "heading": 045.0
      },
      "threat_level": "high",
      "state": "active",
      "first_seen": "2024-01-15T10:00:00Z",
      "last_updated": "2024-01-15T10:30:00Z",
      "detection_count": 42,
      "sources": ["sensor-001", "sensor-003"],
      "pending_proposals": 1
    }
  ],
  "total": 1,
  "limit": 10,
  "offset": 0
}
```

---

#### GET /api/v1/tracks/:id

Get detailed information for a specific track.

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | UUID | Track ID |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/tracks/550e8400-e29b-41d4-a716-446655440000"
```

**Response**

```json
{
  "track_id": "550e8400-e29b-41d4-a716-446655440000",
  "external_track_id": "TRK-001",
  "classification": "hostile",
  "type": "aircraft",
  "confidence": 0.85,
  "position": {
    "lat": 34.0522,
    "lon": -118.2437,
    "alt": 10000
  },
  "velocity": {
    "speed": 250.5,
    "heading": 045.0
  },
  "threat_level": "high",
  "state": "active",
  "first_seen": "2024-01-15T10:00:00Z",
  "last_updated": "2024-01-15T10:30:00Z",
  "detection_count": 42,
  "sources": ["sensor-001", "sensor-003"],
  "metadata": {
    "last_correlation_id": "abc123"
  },
  "history": {
    "positions": [
      {"lat": 34.0500, "lon": -118.2400, "alt": 9500, "time": "2024-01-15T10:25:00Z"},
      {"lat": 34.0522, "lon": -118.2437, "alt": 10000, "time": "2024-01-15T10:30:00Z"}
    ]
  }
}
```

**Status Codes**

| Code | Description |
|------|-------------|
| 200 | Track found |
| 404 | Track not found |

---

### Proposals

#### GET /api/v1/proposals

List action proposals.

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| status | string | pending | Filter: pending, approved, denied, expired |
| track_id | string | - | Filter by track ID |
| action_type | string | - | Filter: engage, track, identify, ignore, intercept, monitor |
| threat_level | string | - | Filter by threat level |
| limit | int | 50 | Maximum results |
| offset | int | 0 | Pagination offset |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/proposals?status=pending&priority_min=5"
```

**Response**

```json
{
  "proposals": [
    {
      "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
      "track_id": "550e8400-e29b-41d4-a716-446655440000",
      "action_type": "intercept",
      "priority": 8,
      "rationale": "High-threat hostile aircraft approaching restricted airspace. Recommend intercept to establish visual identification.",
      "threat_level": "high",
      "status": "pending",
      "hit_count": 3,
      "last_hit_at": "2024-01-15T10:31:00Z",
      "expires_at": "2024-01-15T10:35:00Z",
      "created_at": "2024-01-15T10:30:00Z",
      "track": {
        "track_id": "550e8400-e29b-41d4-a716-446655440000",
        "classification": "hostile",
        "type": "aircraft",
        "threat_level": "high",
        "confidence": 0.85
      }
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

---

#### GET /api/v1/proposals/:id

Get detailed information for a specific proposal.

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | UUID | Proposal ID |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/proposals/660e8400-e29b-41d4-a716-446655440001"
```

**Response**

Same as single proposal in list response, plus:

```json
{
  "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
  "...": "...",
  "track": {
    "track_id": "550e8400-e29b-41d4-a716-446655440000",
    "external_track_id": "TRK-001",
    "classification": "hostile",
    "type": "aircraft",
    "confidence": 0.85,
    "position": {...},
    "velocity": {...},
    "threat_level": "high",
    "detection_count": 42
  }
}
```

---

#### POST /api/v1/proposals/:id/decide

Make a decision (approve or deny) on an action proposal.

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | UUID | Proposal ID |

**Request Body**

```json
{
  "approved": true,
  "approved_by": "operator-001",
  "reason": "Verified threat, proceeding with intercept.",
  "conditions": ["Maintain safe distance", "Report on contact"]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| approved | boolean | Yes | Whether to approve (true) or deny (false) |
| approved_by | string | Yes | Operator identifier |
| reason | string | Yes | Justification for the decision |
| conditions | string[] | No | Additional conditions (for approvals) |

**Request (Approve)**

```bash
curl -X POST "http://localhost:8080/api/v1/proposals/660e8400-e29b-41d4-a716-446655440001/decide" \
  -H "Content-Type: application/json" \
  -d '{
    "approved": true,
    "approved_by": "operator-001",
    "reason": "Verified threat, proceeding with intercept."
  }'
```

**Request (Deny)**

```bash
curl -X POST "http://localhost:8080/api/v1/proposals/660e8400-e29b-41d4-a716-446655440001/decide" \
  -H "Content-Type: application/json" \
  -d '{
    "approved": false,
    "approved_by": "operator-001",
    "reason": "Insufficient confidence. Continue monitoring."
  }'
```

**Response**

```json
{
  "decision_id": "770e8400-e29b-41d4-a716-446655440002",
  "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
  "approved": true,
  "approved_by": "operator-001",
  "approved_at": "2024-01-15T10:32:00Z",
  "reason": "Verified threat, proceeding with intercept.",
  "conditions": ["Maintain safe distance", "Report on contact"]
}
```

**Status Codes**

| Code | Description |
|------|-------------|
| 200 | Decision recorded |
| 400 | Invalid request (missing approved_by, approved field, etc.) |
| 404 | Proposal not found |
| 409 | Proposal already decided or expired |

---

### Decisions

#### GET /api/v1/decisions

List human decisions.

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| approved | bool | - | Filter by approval status |
| approved_by | string | - | Filter by operator |
| since | datetime | - | Decisions after this time |
| limit | int | 100 | Maximum results |
| offset | int | 0 | Pagination offset |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/decisions?approved=true&limit=10"
```

**Response**

```json
{
  "decisions": [
    {
      "decision_id": "770e8400-e29b-41d4-a716-446655440002",
      "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
      "track_id": "550e8400-e29b-41d4-a716-446655440000",
      "action_type": "intercept",
      "approved": true,
      "approved_by": "operator-001",
      "approved_at": "2024-01-15T10:32:00Z",
      "reason": "Verified threat, proceeding with intercept.",
      "conditions": ["Maintain safe distance"],
      "effect_status": "executed"
    }
  ],
  "total": 1,
  "limit": 10,
  "offset": 0
}
```

---

### Effects

#### GET /api/v1/effects

List executed effects.

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| status | string | - | Filter: pending, executed, failed, simulated |
| action_type | string | - | Filter by action type |
| since | datetime | - | Effects after this time |
| limit | int | 100 | Maximum results |
| offset | int | 0 | Pagination offset |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/effects?status=executed"
```

**Response**

```json
{
  "effects": [
    {
      "effect_id": "880e8400-e29b-41d4-a716-446655440004",
      "decision_id": "770e8400-e29b-41d4-a716-446655440002",
      "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
      "track_id": "550e8400-e29b-41d4-a716-446655440000",
      "action_type": "intercept",
      "status": "executed",
      "executed_at": "2024-01-15T10:32:05Z",
      "result": "Intercept order dispatched",
      "idempotent_key": "sha256:abc123...",
      "created_at": "2024-01-15T10:32:00Z"
    }
  ],
  "total": 1,
  "limit": 100,
  "offset": 0
}
```

---

### Audit Trail

#### GET /api/v1/audit

List audit trail entries.

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| limit | int | 100 | Maximum results to return |
| action_type | string | - | Filter by action type |
| user_id | string | - | Filter by user/operator ID |
| track_id | string | - | Filter by track ID |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/audit?limit=50"
```

**Response**

```json
[
  {
    "id": "audit-001",
    "timestamp": "2024-01-15T10:32:00Z",
    "action_type": "decision.approved",
    "user_id": "operator-001",
    "track_id": "550e8400-e29b-41d4-a716-446655440000",
    "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
    "decision_id": "770e8400-e29b-41d4-a716-446655440002",
    "details": "Approved intercept action for hostile aircraft",
    "reason": "Verified threat, proceeding with intercept."
  }
]
```

---

### Track History

#### GET /api/v1/tracks/:id/history

Get detection history for a specific track.

**Path Parameters**

| Parameter | Type | Description |
|-----------|------|-------------|
| id | string | Track ID |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/tracks/TRK-001/history"
```

**Response**

```json
{
  "track_id": "TRK-001",
  "history": [
    {
      "timestamp": "2024-01-15T10:25:00Z",
      "position": {"lat": 34.0500, "lon": -118.2400, "alt": 9500},
      "confidence": 0.82
    },
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "position": {"lat": 34.0522, "lon": -118.2437, "alt": 10000},
      "confidence": 0.85
    }
  ]
}
```

---

### System Metrics

#### GET /api/v1/metrics

Get aggregated system metrics.

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/metrics"
```

**Response**

```json
{
  "active_tracks": 10,
  "pending_proposals": 2,
  "approved_decisions": 15,
  "denied_decisions": 3,
  "executed_effects": 12,
  "messages_per_minute": 500,
  "queue_depth": {
    "detections": 5,
    "tracks": 3,
    "proposals": 2,
    "decisions": 1
  }
}
```

---

#### GET /api/v1/metrics/stages

Get per-stage pipeline metrics.

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/metrics/stages"
```

**Response**

```json
{
  "stages": [
    {
      "name": "Sensor",
      "messages_processed": 1000,
      "latency_p50_ms": 5,
      "latency_p95_ms": 12,
      "latency_p99_ms": 25
    },
    {
      "name": "Classifier",
      "messages_processed": 998,
      "latency_p50_ms": 8,
      "latency_p95_ms": 20,
      "latency_p99_ms": 45
    }
  ]
}
```

---

#### GET /api/v1/metrics/latency

Get end-to-end latency metrics with configurable time window.

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| window | string | 5m | Time window for latency calculation (e.g., 1m, 5m, 15m) |

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/metrics/latency?window=5m"
```

**Response**

```json
{
  "window": "5m",
  "detection_to_decision": {
    "p50_ms": 450,
    "p95_ms": 1200,
    "p99_ms": 2500
  },
  "decision_to_effect": {
    "p50_ms": 50,
    "p95_ms": 150,
    "p99_ms": 300
  },
  "end_to_end": {
    "p50_ms": 500,
    "p95_ms": 1500,
    "p99_ms": 2800
  }
}
```

---

### Classifier Configuration

#### GET /api/v1/classifier/config

Get the current classifier configuration.

**Request**

```bash
curl -X GET "http://localhost:8080/api/v1/classifier/config"
```

**Response**

```json
{
  "classification_weights": {
    "friendly": 25,
    "hostile": 25,
    "neutral": 25,
    "unknown": 25
  },
  "type_weights": {
    "aircraft": 30,
    "vessel": 20,
    "ground": 20,
    "missile": 10,
    "unknown": 20
  }
}
```

---

#### PATCH /api/v1/classifier/config

Update classifier configuration.

**Request Body**

```json
{
  "classification_weights": {
    "friendly": 20,
    "hostile": 40,
    "neutral": 20,
    "unknown": 20
  }
}
```

**Request**

```bash
curl -X PATCH "http://localhost:8080/api/v1/classifier/config" \
  -H "Content-Type: application/json" \
  -d '{"classification_weights": {"friendly": 20, "hostile": 40, "neutral": 20, "unknown": 20}}'
```

**Response**

```json
{
  "success": true,
  "config": {
    "classification_weights": {
      "friendly": 20,
      "hostile": 40,
      "neutral": 20,
      "unknown": 20
    }
  }
}
```

---

### Database Management

#### POST /api/v1/clear

Clear all data from the database (for testing/development).

> **Warning:** This endpoint deletes all tracks, proposals, decisions, effects, and audit entries.

**Request**

```bash
curl -X POST "http://localhost:8080/api/v1/clear"
```

**Response**

```json
{
  "deleted": {
    "tracks": 50,
    "proposals": 12,
    "decisions": 10,
    "effects": 8,
    "audit_entries": 100
  }
}
```

---

### Prometheus Metrics

#### GET /metrics

Prometheus-compatible metrics endpoint.

**Request**

```bash
curl -X GET "http://localhost:8080/metrics"
```

**Response**

```
# HELP api_requests_total Total API requests
# TYPE api_requests_total counter
api_requests_total{method="GET",endpoint="/api/v1/tracks",status="200"} 42
api_requests_total{method="POST",endpoint="/api/v1/proposals/:id/approve",status="200"} 5

# HELP api_request_duration_seconds API request duration
# TYPE api_request_duration_seconds histogram
api_request_duration_seconds_bucket{endpoint="/api/v1/tracks",le="0.01"} 35
api_request_duration_seconds_bucket{endpoint="/api/v1/tracks",le="0.05"} 40
...
```

---

## WebSocket API

### Connection

```
ws://localhost:8080/ws
```

### Message Format

All WebSocket messages use this envelope:

```json
{
  "type": "message_type",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": { ... }
}
```

### Message Types

#### Server -> Client

##### track.new

Sent when a new track is first detected.

```json
{
  "type": "track.new",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "track_id": "550e8400-e29b-41d4-a716-446655440000",
    "classification": "hostile",
    "type": "aircraft",
    "confidence": 0.85,
    "position": {
      "lat": 34.0522,
      "lon": -118.2437,
      "alt": 10000
    },
    "threat_level": "high"
  }
}
```

##### track.update

Sent when an existing track is updated.

```json
{
  "type": "track.update",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "track_id": "550e8400-e29b-41d4-a716-446655440000",
    "classification": "hostile",
    "type": "aircraft",
    "confidence": 0.85,
    "position": {
      "lat": 34.0522,
      "lon": -118.2437,
      "alt": 10000
    },
    "velocity": {
      "speed": 250.5,
      "heading": 045.0
    },
    "threat_level": "high",
    "detection_count": 42,
    "last_updated": "2024-01-15T10:30:00Z"
  }
}
```

##### proposal.new

Sent when a new proposal is created or an existing proposal is updated with a new sensor hit.

```json
{
  "type": "proposal.new",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
    "track_id": "550e8400-e29b-41d4-a716-446655440000",
    "action_type": "intercept",
    "priority": 8,
    "rationale": "High-threat hostile aircraft...",
    "threat_level": "high",
    "hit_count": 3,
    "expires_at": "2024-01-15T10:35:00Z"
  }
}
```

##### decision.made

Sent when a human makes a decision.

```json
{
  "type": "decision.made",
  "timestamp": "2024-01-15T10:32:00Z",
  "data": {
    "decision_id": "770e8400-e29b-41d4-a716-446655440002",
    "proposal_id": "660e8400-e29b-41d4-a716-446655440001",
    "approved": true,
    "approved_by": "operator-001"
  }
}
```

##### effect.executed

Sent when an effect is executed.

```json
{
  "type": "effect.executed",
  "timestamp": "2024-01-15T10:32:05Z",
  "data": {
    "effect_id": "880e8400-e29b-41d4-a716-446655440004",
    "decision_id": "770e8400-e29b-41d4-a716-446655440002",
    "action_type": "intercept",
    "status": "executed",
    "result": "Intercept order dispatched"
  }
}
```

##### metrics.update

Periodic system metrics broadcast (sent every few seconds).

```json
{
  "type": "metrics.update",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {
    "active_tracks": 10,
    "pending_proposals": 2,
    "approved_decisions": 15,
    "denied_decisions": 3,
    "executed_effects": 12,
    "messages_per_minute": 500,
    "queue_depth": {
      "detections": 5,
      "tracks": 3,
      "proposals": 2,
      "decisions": 1
    }
  }
}
```

#### Client -> Server

##### subscribe

Subscribe to specific message types. If no subscriptions are set, the client receives all message types.

**Available topics:** `track.new`, `track.update`, `proposal.new`, `decision.made`, `effect.executed`, `metrics.update`

```json
{
  "type": "subscribe",
  "data": {
    "topics": ["track.new", "track.update", "proposal.new", "decision.made", "metrics.update"]
  }
}
```

##### unsubscribe

Unsubscribe from message types.

```json
{
  "type": "unsubscribe",
  "data": {
    "topics": ["track.update"]
  }
}
```

##### ping

Keep-alive ping.

```json
{
  "type": "ping",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Server response:**

```json
{
  "type": "pong",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

## Error Responses

All error responses follow this format:

```json
{
  "error": {
    "code": "PROPOSAL_NOT_FOUND",
    "message": "Proposal with ID 660e8400-e29b-41d4-a716-446655440001 not found",
    "details": {
      "proposal_id": "660e8400-e29b-41d4-a716-446655440001"
    }
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| VALIDATION_ERROR | 400 | Request validation failed |
| MISSING_FIELD | 400 | Required field missing |
| INVALID_UUID | 400 | Invalid UUID format |
| TRACK_NOT_FOUND | 404 | Track does not exist |
| PROPOSAL_NOT_FOUND | 404 | Proposal does not exist |
| DECISION_NOT_FOUND | 404 | Decision does not exist |
| EFFECT_NOT_FOUND | 404 | Effect does not exist |
| PROPOSAL_EXPIRED | 409 | Proposal has expired |
| PROPOSAL_ALREADY_DECIDED | 409 | Proposal already has a decision |
| POLICY_DENIED | 403 | OPA policy denied the action |
| DATABASE_ERROR | 500 | Database operation failed |
| NATS_ERROR | 500 | Message broker error |

---

## Rate Limiting

> Note: Rate limiting is not implemented in the MVP.

Future implementation will include:
- Per-IP rate limits
- Per-user rate limits
- Endpoint-specific limits

---

## Pagination

List endpoints support pagination using `limit` and `offset` parameters.

**Response includes:**

```json
{
  "items": [...],
  "total": 150,
  "limit": 50,
  "offset": 0,
  "has_more": true
}
```

**Example: Page through results**

```bash
# Page 1
curl "http://localhost:8080/api/v1/tracks?limit=50&offset=0"

# Page 2
curl "http://localhost:8080/api/v1/tracks?limit=50&offset=50"

# Page 3
curl "http://localhost:8080/api/v1/tracks?limit=50&offset=100"
```
