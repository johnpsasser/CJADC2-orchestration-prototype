// Types matching the Go message definitions in pkg/messages

// Envelope contains metadata common to all messages for tracing and security
export interface Envelope {
  message_id: string;
  correlation_id: string;
  causation_id: string;
  source: string;
  source_type: string;
  timestamp: string;
  signature: string;
  policy_version: string;
  trace_id?: string;
  span_id?: string;
}

// Position represents a geographic position
export interface Position {
  lat: number;
  lon: number;
  alt: number;
}

// Velocity represents speed and direction
export interface Velocity {
  speed: number;
  heading: number;
}

// PolicyDecision captures an OPA policy evaluation result
export interface PolicyDecision {
  allowed: boolean;
  reasons?: string[];
  violations?: string[];
  warnings?: string[];
  metadata?: Record<string, string>;
}

// Detection represents a raw sensor detection event
export interface Detection {
  envelope: Envelope;
  track_id: string;
  position: Position;
  velocity: Velocity;
  confidence: number;
  sensor_type: string;
  sensor_id: string;
  raw_data?: string;
}

// Track represents a classified and enriched track
export interface Track {
  envelope: Envelope;
  track_id: string;
  classification: 'friendly' | 'hostile' | 'unknown' | 'neutral';
  type: 'aircraft' | 'vessel' | 'ground' | 'missile' | 'unknown';
  position: Position;
  velocity: Velocity;
  confidence: number;
  first_seen: string;
  last_updated: string;
  detection_count: number;
  sources: string[];
}

// CorrelatedTrack represents a track after correlation/deduplication
export interface CorrelatedTrack {
  envelope: Envelope;
  track_id: string;
  merged_from: string[];
  classification: 'friendly' | 'hostile' | 'unknown' | 'neutral';
  type: 'aircraft' | 'vessel' | 'ground' | 'missile' | 'unknown';
  position: Position;
  velocity: Velocity;
  confidence: number;
  threat_level: ThreatLevel;
  window_start: string;
  window_end: string;
  last_updated: string;
  detection_count: number;
  sources: string[];
  [key: string]: unknown; // Index signature for compatibility
}

// ThreatLevel enum
export type ThreatLevel = 'critical' | 'high' | 'medium' | 'low' | 'unknown';

// ActionType enum
export type ActionType = 'engage' | 'track' | 'identify' | 'ignore' | 'intercept' | 'monitor';

// ActionProposal represents a proposed action requiring human approval
export interface ActionProposal {
  envelope?: Envelope; // Optional - not returned by REST API
  proposal_id: string;
  track_id: string;
  action_type: ActionType;
  priority: number;
  rationale: string;
  constraints?: string[];
  track?: CorrelatedTrack;
  threat_level: ThreatLevel;
  expires_at: string;
  policy_decision: PolicyDecision;
  status?: string; // Added - returned by backend
  created_at?: string; // Added - returned by backend
}

// Decision represents a human decision on an action proposal
export interface Decision {
  envelope?: Envelope; // Optional - not returned by REST API
  decision_id: string;
  proposal_id: string;
  approved: boolean;
  approved_by: string;
  approved_at: string;
  reason?: string;
  conditions?: string[];
  action_type?: ActionType; // Optional - may not be in API response
  track_id?: string; // Optional - may not be in API response
}

// EffectLog represents the execution of an approved action
export interface EffectLog {
  envelope: Envelope;
  effect_id: string;
  decision_id: string;
  proposal_id: string;
  track_id: string;
  action_type: ActionType;
  status: 'executed' | 'failed' | 'simulated' | 'pending';
  executed_at: string;
  result: string;
  idempotent_key: string;
  idempotent: boolean;
}

// WebSocket message types
export type WSMessageType =
  | 'track.update'
  | 'track.new'
  | 'track.delete'
  | 'proposal.new'
  | 'proposal.update'
  | 'proposal.expired'
  | 'decision.made'
  | 'effect.executed'
  | 'metrics.update'
  | 'connection.status'
  | 'ping'
  | 'pong';

export interface WSMessage<T = unknown> {
  type: WSMessageType;
  payload: T;
  timestamp: string;
}

// Metrics types
export interface StageMetrics {
  stage: string;
  processed: number;
  succeeded: number;
  failed: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
  latency_p99_ms: number;
  last_updated: string;
}

export interface SystemMetrics {
  stages: StageMetrics[];
  end_to_end_latency_p50_ms: number;
  end_to_end_latency_p95_ms: number;
  end_to_end_latency_p99_ms: number;
  messages_per_minute: number;
  active_tracks: number;
  pending_proposals: number;
  timestamp: string;
}

export interface LatencyDataPoint {
  timestamp: string;
  p50: number;
  p95: number;
  p99: number;
}

// Audit types
export interface AuditEntry {
  id: string;
  timestamp: string;
  action_type: ActionType;
  user_id?: string;
  track_id: string;
  proposal_id?: string;
  decision_id?: string;
  effect_id?: string;
  status: 'proposed' | 'approved' | 'denied' | 'executed' | 'failed';
  details: string;
}

// API response types
export interface APIResponse<T> {
  data: T;
  correlation_id: string;
  timestamp: string;
}

export interface APIError {
  error: string;        // Error type (e.g., "bad_request", "not_found")
  message: string;      // Human-readable error message
  correlation_id: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  page_size: number;
  correlation_id: string;
}

// Decision request type
export interface DecisionRequest {
  proposal_id: string;
  approved: boolean;
  approved_by: string;
  reason: string;
  conditions?: string[];
}

// Sort configuration
export interface SortConfig {
  key: string;
  direction: 'asc' | 'desc';
}

// Connection status
export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'error';

// Track type weights for sensor configuration
export interface TrackTypeWeights {
  aircraft: number;
  vessel: number;
  ground: number;
  missile: number;
  unknown: number;
}

// Classification weights for sensor configuration
export interface ClassificationWeights {
  friendly: number;
  hostile: number;
  neutral: number;
  unknown: number;
}
