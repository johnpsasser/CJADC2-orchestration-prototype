import { v4 as uuidv4 } from 'uuid';

// Classifier configuration types
export interface ClassifierConfig {
  paused: boolean;
}

const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

// Classifier control API endpoints (via API gateway)
export const classifierApi = {
  // Get current classifier configuration
  getConfig: async (): Promise<ClassifierConfig> => {
    const response = await fetch(`${API_BASE_URL}/api/v1/classifier/config`, {
      headers: {
        'Content-Type': 'application/json',
        'X-Correlation-ID': uuidv4(),
      },
    });
    if (!response.ok) {
      throw new Error(`Failed to get classifier config: ${response.statusText}`);
    }
    return response.json();
  },

  // Update classifier configuration (partial update)
  updateConfig: async (config: Partial<ClassifierConfig>): Promise<ClassifierConfig> => {
    const response = await fetch(`${API_BASE_URL}/api/v1/classifier/config`, {
      method: 'PATCH',
      headers: {
        'Content-Type': 'application/json',
        'X-Correlation-ID': uuidv4(),
      },
      body: JSON.stringify(config),
    });
    if (!response.ok) {
      throw new Error(`Failed to update classifier config: ${response.statusText}`);
    }
    return response.json();
  },
};

export default classifierApi;
