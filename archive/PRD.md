# CJADC2 Machine-to-Machine Orchestration Platform

## Product Requirements Document (PRD)

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-01-15

---

## 1. Executive Summary

The CJADC2 (Combined Joint All-Domain Command and Control) Machine-to-Machine Orchestration Platform is an MVP event-driven system designed to accelerate complex mission decision chains. The platform coordinates six autonomous agents through a secure messaging backbone, treating "Sensor Data as a Product" while maintaining strict human-in-the-loop approval for all effects.

**Mission Chain:** Sensor -> Classify -> Correlate -> Plan -> Request-Approval -> Dispatch

---

## 2. Goals and Non-Goals

### Goals

- **G1:** Demonstrate end-to-end event-driven orchestration from sensor detection to effect execution
- **G2:** Enforce human-in-the-loop approval for all proposed actions
- **G3:** Provide policy enforcement at every processing stage via OPA/Rego
- **G4:** Achieve sub-3-second detection-to-decision latency (p95) on commodity hardware
- **G5:** Support at-least-once message delivery with idempotent handlers
- **G6:** Enable full observability through correlation IDs, metrics, and distributed tracing

### Non-Goals

- **NG1:** Integration with real-world sensor systems or targeting capabilities
- **NG2:** Production-grade high availability or disaster recovery
- **NG3:** Multi-tenant deployment or role-based access control beyond HIL approval
- **NG4:** Real-time streaming analytics or ML model training pipelines
- **NG5:** Mobile or offline-capable client applications

---

## 3. Target Users

### Primary Users

| User Type | Description | Key Needs |
|-----------|-------------|-----------|
| **Mission Operators** | Personnel monitoring the decision chain and approving/denying proposed actions | Clear UI for action review, approval workflow, situational awareness |
| **System Integrators** | Engineers deploying, configuring, and extending the platform | API documentation, configuration management, extensible agent architecture |

### Secondary Users

| User Type | Description | Key Needs |
|-----------|-------------|-----------|
| **DevOps Engineers** | Personnel responsible for deployment and monitoring | Observability dashboards, health checks, log aggregation |
| **Policy Authors** | Personnel defining operational constraints | Rego policy editing, policy testing tools |

---

## 4. Key Performance Indicators (KPIs)

### Latency Requirements

| Metric | Target | Measurement Point |
|--------|--------|-------------------|
| Detection-to-Decision (p50) | < 1.5s | Sensor emission to ActionProposal creation |
| Detection-to-Decision (p95) | < 3.0s | Sensor emission to ActionProposal creation |
| End-to-End (p95) | < 5.0s | Sensor emission to Effector execution (excluding HIL wait) |

### Throughput Requirements

| Metric | Target | Environment |
|--------|--------|-------------|
| Message Processing Rate | >= 1,000 msgs/min | Local laptop deployment |
| Concurrent Tracks | >= 100 | Active tracks in correlation window |
| Proposal Queue Depth | >= 50 | Pending HIL approvals |

### Reliability Requirements

| Metric | Target | Notes |
|--------|--------|-------|
| Message Delivery | At-least-once | Via NATS JetStream store-and-forward |
| Handler Idempotency | 100% | Duplicate messages produce identical outcomes |
| Policy Evaluation | 100% coverage | Every message evaluated against OPA policies |

---

## 5. System Architecture

### Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Messaging | NATS JetStream + mTLS | Secure, persistent pub/sub messaging |
| Policy Engine | OPA/Rego | Declarative policy enforcement |
| Persistence | PostgreSQL | Event store, track state, audit log |
| Agents | Go | High-performance event processors |
| UI | React + WebSocket | Real-time operator dashboard |
| Observability | Prometheus + OpenTelemetry | Metrics, tracing, alerting |

### Agent Pipeline

```
+-------------+    +------------+    +------------+    +---------+    +------------+    +----------+
| Sensor Sim  | -> | Classifier | -> | Correlator | -> | Planner | -> | Authorizer | -> | Effector |
+-------------+    +------------+    +------------+    +---------+    +------------+    +----------+
     |                  |                 |                |               |                 |
     v                  v                 v                v               v                 v
  Detection          Track            Correlated       Action          Approved          Effect
   Event            Event              Track          Proposal         Action           Executed
```

| Agent | Input | Output | Responsibility |
|-------|-------|--------|----------------|
| Sensor Sim | Timer/Config | Detection Event | Emit synthetic detections with random tracks and confidence scores |
| Classifier | Detection | Track Event | Enrich detections with type classification and confidence updates |
| Correlator | Track Event | Correlated Track | Deduplicate and merge tracks within 10-second time windows |
| Planner | Correlated Track | ActionProposal | Generate proposed responses based on track characteristics |
| Authorizer | ActionProposal | Approved Action | Present proposals for human approval/denial via UI |
| Effector | Approved Action | Effect Log | Execute simulated effects and log state transitions |

---

## 6. Threat Model and Safety Constraints

### Threat Model

| Threat Category | Mitigation | MVP Status |
|-----------------|------------|------------|
| **Message Tampering** | mTLS encryption for all NATS communication | ⚠️ Deferred (non-goal NG2) |
| **Unauthorized Access** | Certificate-based authentication for agents | ⚠️ Using HMAC signing instead |
| **Policy Bypass** | Mandatory OPA evaluation at each pipeline stage | ✅ Implemented |
| **Replay Attacks** | Idempotent handlers with deduplication keys | ✅ Implemented |
| **Audit Evasion** | Immutable PostgreSQL audit log with correlation IDs | ✅ Implemented |

> **Note:** mTLS and certificate-based authentication are deferred per non-goal NG2 (production-grade HA). The MVP uses HMAC-SHA256 message signing for integrity verification.

### Safety Constraints

| Constraint | Implementation | Rationale |
|------------|----------------|-----------|
| **Synthetic Data Only** | All sensor data is randomly generated | No connection to real-world systems |
| **No Real-World Targeting** | Effects are log-only simulations | Platform has no external system integration |
| **Human-in-the-Loop Required** | Authorizer agent blocks until operator approval | No autonomous effect execution permitted |
| **Simulated Effects** | Effector writes to log, no external API calls | Effects cannot impact real systems |
| **Audit Trail** | All decisions logged with correlation IDs | Full traceability for review |

### Safety Invariants

1. **No ActionProposal proceeds to Effector without explicit human approval**
2. **All effects are simulated and produce only log entries and state transitions**
3. **Correlation IDs trace every event from sensor to effect for auditability**
4. **Policy violations halt processing and generate alerts**

---

## 7. Observability Requirements

### Metrics Endpoint

Each agent exposes a `/metrics` endpoint with:

- `messages_processed_total` - Counter of processed messages
- `messages_succeeded_total` - Counter of successfully handled messages
- `messages_failed_total` - Counter of failed message handling
- `message_latency_seconds` - Histogram with p50, p95, p99 buckets

### Distributed Tracing

- OpenTelemetry spans for each agent processing step
- Correlation ID propagation through all message headers
- Trace context preserved across NATS publish/subscribe

### Health Checks

- Liveness probe: Agent process running
- Readiness probe: NATS connection established, PostgreSQL reachable

---

## 8. Success Criteria

### MVP Acceptance

- [x] Full pipeline executes from Sensor Sim to Effector with HIL approval
- [x] Detection-to-decision p95 latency < 3 seconds on laptop
- [x] Sustained throughput >= 1,000 messages/minute
- [x] All effects blocked without human approval
- [x] Correlation IDs trace end-to-end for any event
- [x] Prometheus metrics available for all agents
- [x] Policy enforcement demonstrated at each stage

> **Status:** All MVP acceptance criteria have been met. The platform successfully demonstrates end-to-end event-driven orchestration with human-in-the-loop approval.

### Beyond MVP: Additional Capabilities

The following capabilities were implemented beyond the original PRD scope:

| Feature | Description |
|---------|-------------|
| **Sensor Runtime Configuration** | HTTP API for adjusting emission rates, track counts, and type distributions at runtime |
| **Proposal De-duplication** | Consolidates multiple sensor hits on the same track into single proposals with hit counting |
| **Metrics Dashboard** | Real-time pipeline metrics including per-stage latency percentiles and queue depths |
| **Track History API** | Historical detection positions for individual tracks |
| **Audit Trail UI** | Searchable, filterable audit log with grouping and timeline views |
| **Consumer Resilience** | Automatic NATS consumer recreation on failure for improved reliability |
| **Classification Biasing** | Special handling for missile tracks with configurable classification weights |

---

## Appendix A: Glossary

| Term | Definition |
|------|------------|
| **CJADC2** | Combined Joint All-Domain Command and Control |
| **HIL** | Human-in-the-Loop |
| **OPA** | Open Policy Agent |
| **mTLS** | Mutual Transport Layer Security |
| **Correlation ID** | Unique identifier tracing an event through the entire pipeline |
| **ActionProposal** | A suggested response generated by the Planner awaiting human approval |
| **Effect** | The simulated outcome of an approved action |
