import { v4 as uuidv4 } from 'uuid';
import type {
  APIResponse,
  APIError,
  PaginatedResponse,
  CorrelatedTrack,
  ActionProposal,
  Decision,
  DecisionRequest,
  EffectLog,
  SystemMetrics,
  AuditEntry,
  InterventionRule,
  InterventionRuleCreate,
  InterventionRuleUpdate,
} from '../types';

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

// Generate a correlation ID for request tracing
function generateCorrelationId(): string {
  return uuidv4();
}

// Custom error class for API errors
export class APIClientError extends Error {
  code: string;
  correlationId: string;

  constructor(message: string, code: string, correlationId: string) {
    super(message);
    this.name = 'APIClientError';
    this.code = code;
    this.correlationId = correlationId;
  }
}

// Generic fetch wrapper with error handling and correlation ID tracking
async function apiFetch<T>(
  endpoint: string,
  options: RequestInit = {},
  correlationId?: string
): Promise<APIResponse<T>> {
  const corrId = correlationId || generateCorrelationId();

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    'X-Correlation-ID': corrId,
    ...options.headers,
  };

  try {
    const response = await fetch(`${API_BASE_URL}${endpoint}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const errorData: APIError = await response.json().catch(() => ({
        error: `HTTP_${response.status}`,
        message: `HTTP ${response.status}: ${response.statusText}`,
        correlation_id: corrId,
      }));

      throw new APIClientError(
        errorData.message || errorData.error,  // Use message if available, fallback to error type
        errorData.error,                        // Error type/code
        errorData.correlation_id
      );
    }

    const data = await response.json();
    return {
      data: data as T,
      correlation_id: response.headers.get('X-Correlation-ID') || corrId,
      timestamp: new Date().toISOString(),
    };
  } catch (error) {
    if (error instanceof APIClientError) {
      throw error;
    }
    throw new APIClientError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK_ERROR',
      corrId
    );
  }
}

// Track list response from the backend
interface TrackListResponse {
  tracks: CorrelatedTrack[];
  total: number;
  limit: number;
  offset: number;
  correlation_id: string;
}

// Track API endpoints
export const tracksApi = {
  // Get all active tracks
  getAll: async (correlationId?: string): Promise<APIResponse<CorrelatedTrack[]>> => {
    const response = await apiFetch<TrackListResponse>('/api/v1/tracks', {}, correlationId);
    return {
      ...response,
      data: response.data.tracks || [],
    };
  },

  // Get a specific track by ID
  getById: async (
    trackId: string,
    correlationId?: string
  ): Promise<APIResponse<CorrelatedTrack>> => {
    return apiFetch<CorrelatedTrack>(
      `/api/v1/tracks/${encodeURIComponent(trackId)}`,
      {},
      correlationId
    );
  },

  // Get paginated tracks
  getPaginated: async (
    page: number = 1,
    pageSize: number = 50,
    correlationId?: string
  ): Promise<PaginatedResponse<CorrelatedTrack>> => {
    const response = await apiFetch<PaginatedResponse<CorrelatedTrack>>(
      `/api/v1/tracks?page=${page}&page_size=${pageSize}`,
      {},
      correlationId
    );
    return response.data;
  },
};

// Proposal API endpoints
export const proposalsApi = {
  // Get all pending proposals
  getPending: async (correlationId?: string): Promise<APIResponse<ActionProposal[]>> => {
    const response = await apiFetch<{ proposals: ActionProposal[] }>('/api/v1/proposals?status=pending', {}, correlationId);
    return {
      ...response,
      data: response.data.proposals || [],
    };
  },

  // Get a specific proposal by ID
  getById: async (
    proposalId: string,
    correlationId?: string
  ): Promise<APIResponse<ActionProposal>> => {
    return apiFetch<ActionProposal>(
      `/api/v1/proposals/${encodeURIComponent(proposalId)}`,
      {},
      correlationId
    );
  },

  // Submit a decision for a proposal
  submitDecision: async (
    request: DecisionRequest,
    correlationId?: string
  ): Promise<APIResponse<Decision>> => {
    const body = {
      approved: request.approved,
      approved_by: request.approved_by || 'operator', // Default to 'operator' if not set
      reason: request.reason || '',
      conditions: request.conditions,
    };
    console.log('[submitDecision] Request body:', body);
    return apiFetch<Decision>(
      `/api/v1/proposals/${encodeURIComponent(request.proposal_id)}/decide`,
      {
        method: 'POST',
        body: JSON.stringify(body),
      },
      correlationId
    );
  },
};

// Decision API endpoints
export const decisionsApi = {
  // Get recent decisions
  getRecent: async (
    limit: number = 50,
    correlationId?: string
  ): Promise<APIResponse<Decision[]>> => {
    return apiFetch<Decision[]>(
      `/api/v1/decisions?limit=${limit}`,
      {},
      correlationId
    );
  },

  // Get a specific decision by ID
  getById: async (
    decisionId: string,
    correlationId?: string
  ): Promise<APIResponse<Decision>> => {
    return apiFetch<Decision>(
      `/api/v1/decisions/${encodeURIComponent(decisionId)}`,
      {},
      correlationId
    );
  },
};

// Effect API endpoints
export const effectsApi = {
  // Get recent effects
  getRecent: async (
    limit: number = 50,
    correlationId?: string
  ): Promise<APIResponse<EffectLog[]>> => {
    return apiFetch<EffectLog[]>(
      `/api/v1/effects?limit=${limit}`,
      {},
      correlationId
    );
  },

  // Get a specific effect by ID
  getById: async (
    effectId: string,
    correlationId?: string
  ): Promise<APIResponse<EffectLog>> => {
    return apiFetch<EffectLog>(
      `/api/v1/effects/${encodeURIComponent(effectId)}`,
      {},
      correlationId
    );
  },
};

// Metrics API endpoints
export const metricsApi = {
  // Get current system metrics
  getCurrent: async (correlationId?: string): Promise<APIResponse<SystemMetrics>> => {
    return apiFetch<SystemMetrics>('/api/v1/metrics', {}, correlationId);
  },

  // Get metrics history for charts
  getHistory: async (
    minutes: number = 60,
    correlationId?: string
  ): Promise<APIResponse<SystemMetrics[]>> => {
    return apiFetch<SystemMetrics[]>(
      `/api/v1/metrics/history?minutes=${minutes}`,
      {},
      correlationId
    );
  },
};

// Audit API endpoints
export const auditApi = {
  // Get audit trail
  getEntries: async (
    options: {
      limit?: number;
      action_type?: string;
      user_id?: string;
      track_id?: string;
    } = {},
    correlationId?: string
  ): Promise<APIResponse<AuditEntry[]>> => {
    const params = new URLSearchParams();
    if (options.limit) params.set('limit', options.limit.toString());
    if (options.action_type) params.set('action_type', options.action_type);
    if (options.user_id) params.set('user_id', options.user_id);
    if (options.track_id) params.set('track_id', options.track_id);

    const query = params.toString();
    return apiFetch<AuditEntry[]>(
      `/api/v1/audit${query ? `?${query}` : ''}`,
      {},
      correlationId
    );
  },
};

// Health check
export const healthApi = {
  check: async (): Promise<boolean> => {
    try {
      await apiFetch<{ status: string }>('/health');
      return true;
    } catch {
      return false;
    }
  },
};

// Intervention rules API endpoints
export const interventionRulesApi = {
  // Get all intervention rules
  getAll: async (params?: { enabled?: boolean; limit?: number }, correlationId?: string): Promise<APIResponse<InterventionRule[]>> => {
    const searchParams = new URLSearchParams();
    if (params?.enabled !== undefined) {
      searchParams.set('enabled', String(params.enabled));
    }
    if (params?.limit) {
      searchParams.set('limit', String(params.limit));
    }
    const query = searchParams.toString();
    const url = `/api/v1/intervention-rules${query ? `?${query}` : ''}`;
    const response = await apiFetch<{ rules: InterventionRule[] }>(url, {}, correlationId);
    return { ...response, data: response.data.rules || [] };
  },

  // Get a single intervention rule by ID
  getById: async (ruleId: string, correlationId?: string): Promise<APIResponse<InterventionRule>> => {
    return apiFetch<InterventionRule>(
      `/api/v1/intervention-rules/${encodeURIComponent(ruleId)}`,
      {},
      correlationId
    );
  },

  // Create a new intervention rule
  create: async (rule: InterventionRuleCreate, correlationId?: string): Promise<APIResponse<InterventionRule>> => {
    return apiFetch<InterventionRule>(
      '/api/v1/intervention-rules',
      {
        method: 'POST',
        body: JSON.stringify(rule),
      },
      correlationId
    );
  },

  // Update an existing intervention rule
  update: async (ruleId: string, rule: InterventionRuleUpdate, correlationId?: string): Promise<APIResponse<InterventionRule>> => {
    return apiFetch<InterventionRule>(
      `/api/v1/intervention-rules/${encodeURIComponent(ruleId)}`,
      {
        method: 'PUT',
        body: JSON.stringify(rule),
      },
      correlationId
    );
  },

  // Delete an intervention rule
  delete: async (ruleId: string, correlationId?: string): Promise<APIResponse<void>> => {
    return apiFetch<void>(
      `/api/v1/intervention-rules/${encodeURIComponent(ruleId)}`,
      {
        method: 'DELETE',
      },
      correlationId
    );
  },
};

// Clear all data response
interface ClearAllResponse {
  success: boolean;
  message: string;
  deleted: {
    tracks: number;
    proposals: number;
    decisions: number;
    effects: number;
    detections: number;
  };
  correlation_id: string;
}

// Clear API endpoints
export const clearApi = {
  // Clear all tracks, proposals, decisions, and effects
  clearAll: async (correlationId?: string): Promise<APIResponse<ClearAllResponse>> => {
    return apiFetch<ClearAllResponse>(
      '/api/v1/clear',
      {
        method: 'POST',
      },
      correlationId
    );
  },
};

// Export all APIs as a single object
export const api = {
  tracks: tracksApi,
  proposals: proposalsApi,
  decisions: decisionsApi,
  effects: effectsApi,
  metrics: metricsApi,
  audit: auditApi,
  health: healthApi,
  clear: clearApi,
  interventionRules: interventionRulesApi,
};

export default api;
