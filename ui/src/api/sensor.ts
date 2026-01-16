import { v4 as uuidv4 } from 'uuid';

// Sensor configuration types
export interface SensorConfig {
  emission_interval_ms: number;
  track_count: number;
  paused: boolean;
}

// Sensor API base URL (sensor-sim runs on port 9090)
const SENSOR_API_BASE_URL = import.meta.env.VITE_SENSOR_API_URL || 'http://localhost:9090';

// Generate a correlation ID for request tracing
function generateCorrelationId(): string {
  return uuidv4();
}

// Custom error class for sensor API errors
export class SensorAPIError extends Error {
  code: string;
  correlationId: string;

  constructor(message: string, code: string, correlationId: string) {
    super(message);
    this.name = 'SensorAPIError';
    this.code = code;
    this.correlationId = correlationId;
  }
}

// Generic fetch wrapper for sensor API
async function sensorFetch<T>(
  endpoint: string,
  options: RequestInit = {},
  correlationId?: string
): Promise<{ data: T; correlationId: string }> {
  const corrId = correlationId || generateCorrelationId();

  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    'X-Correlation-ID': corrId,
    ...options.headers,
  };

  try {
    const response = await fetch(`${SENSOR_API_BASE_URL}${endpoint}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({
        error: `HTTP ${response.status}: ${response.statusText}`,
        code: `HTTP_${response.status}`,
      }));

      throw new SensorAPIError(
        errorData.error || errorData.message || 'Unknown error',
        errorData.code || `HTTP_${response.status}`,
        corrId
      );
    }

    const data = await response.json();
    return {
      data: data as T,
      correlationId: response.headers.get('X-Correlation-ID') || corrId,
    };
  } catch (error) {
    if (error instanceof SensorAPIError) {
      throw error;
    }
    throw new SensorAPIError(
      error instanceof Error ? error.message : 'Network error',
      'NETWORK_ERROR',
      corrId
    );
  }
}

// Sensor control API endpoints
export const sensorApi = {
  // Get current sensor configuration
  getConfig: async (correlationId?: string): Promise<{ data: SensorConfig; correlationId: string }> => {
    return sensorFetch<SensorConfig>('/api/v1/config', {}, correlationId);
  },

  // Update sensor configuration (partial update)
  updateConfig: async (
    config: Partial<SensorConfig>,
    correlationId?: string
  ): Promise<{ data: SensorConfig; correlationId: string }> => {
    return sensorFetch<SensorConfig>(
      '/api/v1/config',
      {
        method: 'PATCH',
        body: JSON.stringify(config),
      },
      correlationId
    );
  },

  // Reset sensor configuration to defaults
  resetConfig: async (correlationId?: string): Promise<{ data: SensorConfig; correlationId: string }> => {
    return sensorFetch<SensorConfig>(
      '/api/v1/config/reset',
      {
        method: 'POST',
      },
      correlationId
    );
  },

  // Pause the sensor
  pause: async (correlationId?: string): Promise<{ data: SensorConfig; correlationId: string }> => {
    return sensorFetch<SensorConfig>(
      '/api/v1/config',
      {
        method: 'PATCH',
        body: JSON.stringify({ paused: true }),
      },
      correlationId
    );
  },

  // Resume the sensor
  resume: async (correlationId?: string): Promise<{ data: SensorConfig; correlationId: string }> => {
    return sensorFetch<SensorConfig>(
      '/api/v1/config',
      {
        method: 'PATCH',
        body: JSON.stringify({ paused: false }),
      },
      correlationId
    );
  },
};

export default sensorApi;
