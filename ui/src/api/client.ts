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
        error: `HTTP ${response.status}: ${response.statusText}`,
        code: `HTTP_${response.status}`,
        correlation_id: corrId,
        timestamp: new Date().toISOString(),
      }));

      throw new APIClientError(
        errorData.error,
        errorData.code,
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
    return apiFetch<Decision>(
      `/api/v1/proposals/${encodeURIComponent(request.proposal_id)}/decide`,
      {
        method: 'POST',
        body: JSON.stringify({
          approved: request.approved,
          reason: request.reason,
          conditions: request.conditions,
        }),
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

// Export all APIs as a single object
export const api = {
  tracks: tracksApi,
  proposals: proposalsApi,
  decisions: decisionsApi,
  effects: effectsApi,
  metrics: metricsApi,
  audit: auditApi,
  health: healthApi,
};

export default api;
