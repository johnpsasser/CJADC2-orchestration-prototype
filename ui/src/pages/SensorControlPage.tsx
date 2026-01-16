import { useState, useEffect, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import clsx from 'clsx';
import { sensorApi, SensorConfig, SensorAPIError } from '../api/sensor';
import { clearApi, APIClientError } from '../api/client';
import type { TrackTypeWeights, ClassificationWeights } from '../types';

// Toast notification component
interface ToastProps {
  message: string;
  type: 'success' | 'error';
  onClose: () => void;
}

function Toast({ message, type, onClose }: ToastProps) {
  useEffect(() => {
    const timer = setTimeout(onClose, 4000);
    return () => clearTimeout(timer);
  }, [onClose]);

  return (
    <div
      className={clsx(
        'fixed bottom-4 right-4 px-4 py-3 rounded-lg shadow-lg flex items-center gap-3 z-50 animate-slide-up',
        type === 'success' ? 'bg-green-900/90 text-green-200 border border-green-700' : 'bg-red-900/90 text-red-200 border border-red-700'
      )}
    >
      <svg
        className={clsx('w-5 h-5', type === 'success' ? 'text-green-400' : 'text-red-400')}
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        {type === 'success' ? (
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
        ) : (
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
        )}
      </svg>
      <span className="text-sm font-medium">{message}</span>
      <button
        onClick={onClose}
        className="ml-2 text-gray-400 hover:text-gray-200"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  );
}

// Status indicator component
function StatusIndicator({ paused, isLoading }: { paused: boolean; isLoading: boolean }) {
  if (isLoading) {
    return (
      <div className="flex items-center gap-2">
        <div className="w-3 h-3 bg-yellow-500 rounded-full animate-pulse" />
        <span className="text-sm text-yellow-400 font-medium">Loading...</span>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <div
        className={clsx(
          'w-3 h-3 rounded-full',
          paused ? 'bg-yellow-500' : 'bg-green-500 animate-pulse'
        )}
      />
      <span
        className={clsx(
          'text-sm font-medium',
          paused ? 'text-yellow-400' : 'text-green-400'
        )}
      >
        {paused ? 'Paused' : 'Running'}
      </span>
    </div>
  );
}

// Slider component for emission interval
interface SliderProps {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  unit: string;
  disabled?: boolean;
  formatValue?: (value: number) => string;
  onChange: (value: number) => void;
}

function Slider({
  label,
  value,
  min,
  max,
  step,
  unit,
  disabled,
  formatValue,
  onChange,
}: SliderProps) {
  const displayValue = formatValue ? formatValue(value) : value.toString();

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium text-gray-300">{label}</label>
        <span className="text-sm font-mono text-green-400">
          {displayValue} {unit}
        </span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(Number(e.target.value))}
        className={clsx(
          'w-full h-2 rounded-lg appearance-none cursor-pointer',
          'bg-gray-700 accent-green-500',
          disabled && 'opacity-50 cursor-not-allowed'
        )}
      />
      <div className="flex justify-between text-xs text-gray-500">
        <span>{formatValue ? formatValue(min) : min} {unit}</span>
        <span>{formatValue ? formatValue(max) : max} {unit}</span>
      </div>
    </div>
  );
}

// Number input component
interface NumberInputProps {
  label: string;
  value: number;
  min: number;
  max: number;
  disabled?: boolean;
  onChange: (value: number) => void;
}

function NumberInput({ label, value, min, max, disabled, onChange }: NumberInputProps) {
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <label className="text-sm font-medium text-gray-300">{label}</label>
        <span className="text-xs text-gray-500">Range: {min} - {max}</span>
      </div>
      <input
        type="number"
        min={min}
        max={max}
        value={value}
        disabled={disabled}
        onChange={(e) => {
          const val = Number(e.target.value);
          if (val >= min && val <= max) {
            onChange(val);
          }
        }}
        className={clsx(
          'w-full px-4 py-2 rounded-lg bg-gray-700 border border-gray-600',
          'text-gray-200 font-mono text-lg',
          'focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-transparent',
          disabled && 'opacity-50 cursor-not-allowed'
        )}
      />
    </div>
  );
}

// Collapsible section component
interface CollapsibleSectionProps {
  title: string;
  isOpen: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}

function CollapsibleSection({ title, isOpen, onToggle, children }: CollapsibleSectionProps) {
  return (
    <div className="border border-gray-700 rounded-lg overflow-hidden">
      <button
        type="button"
        onClick={onToggle}
        className="w-full px-4 py-3 flex items-center justify-between bg-gray-900 hover:bg-gray-800 transition-colors"
      >
        <span className="text-sm font-medium text-gray-300">{title}</span>
        <svg
          className={clsx(
            'w-5 h-5 text-gray-400 transition-transform duration-200',
            isOpen && 'rotate-180'
          )}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      <div
        className={clsx(
          'transition-all duration-200 overflow-hidden',
          isOpen ? 'max-h-[500px] opacity-100' : 'max-h-0 opacity-0'
        )}
      >
        <div className="p-4 bg-gray-800/50">
          {children}
        </div>
      </div>
    </div>
  );
}

// Weight slider with inline numeric input
interface WeightSliderProps {
  label: string;
  value: number;
  disabled?: boolean;
  color: string;
  onChange: (value: number) => void;
}

function WeightSlider({ label, value, disabled, color, onChange }: WeightSliderProps) {
  return (
    <div className="flex items-center gap-4">
      <div className="w-24 flex-shrink-0">
        <span className="text-sm text-gray-300 capitalize">{label}</span>
      </div>
      <div className="flex-1">
        <input
          type="range"
          min={0}
          max={100}
          step={1}
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(Number(e.target.value))}
          className={clsx(
            'w-full h-2 rounded-lg appearance-none cursor-pointer bg-gray-700',
            color,
            disabled && 'opacity-50 cursor-not-allowed'
          )}
        />
      </div>
      <div className="w-20 flex-shrink-0">
        <input
          type="number"
          min={0}
          max={100}
          value={value}
          disabled={disabled}
          onChange={(e) => {
            const val = Math.min(100, Math.max(0, Number(e.target.value) || 0));
            onChange(val);
          }}
          className={clsx(
            'w-full px-2 py-1 rounded bg-gray-700 border border-gray-600',
            'text-gray-200 font-mono text-sm text-right',
            'focus:outline-none focus:ring-1 focus:ring-green-500 focus:border-transparent',
            disabled && 'opacity-50 cursor-not-allowed'
          )}
        />
      </div>
      <span className="text-xs text-gray-500 w-4">%</span>
    </div>
  );
}

// Default weight values
const DEFAULT_TYPE_WEIGHTS: TrackTypeWeights = {
  aircraft: 40,
  vessel: 20,
  ground: 15,
  missile: 5,
  unknown: 20,
};

const DEFAULT_CLASSIFICATION_WEIGHTS: ClassificationWeights = {
  friendly: 30,
  hostile: 25,
  neutral: 20,
  unknown: 25,
};

// Main SensorControlPage component
export function SensorControlPage() {
  const queryClient = useQueryClient();
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Local state for form values (optimistic UI)
  const [localConfig, setLocalConfig] = useState<SensorConfig | null>(null);
  const [hasChanges, setHasChanges] = useState(false);

  // Collapsible section state
  const [typeWeightsOpen, setTypeWeightsOpen] = useState(false);
  const [classificationWeightsOpen, setClassificationWeightsOpen] = useState(false);

  // Fetch current configuration
  const { data: configData, isLoading, error, refetch } = useQuery({
    queryKey: ['sensorConfig'],
    queryFn: async () => {
      const response = await sensorApi.getConfig();
      return response.data;
    },
    refetchInterval: 5000, // Poll every 5 seconds
    staleTime: 3000,
  });

  // Sync local state with fetched config
  useEffect(() => {
    if (configData && !hasChanges) {
      setLocalConfig(configData);
    }
  }, [configData, hasChanges]);

  // Update configuration mutation
  const updateMutation = useMutation({
    mutationFn: async (config: Partial<SensorConfig>) => {
      const response = await sensorApi.updateConfig(config);
      return response.data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(['sensorConfig'], data);
      setLocalConfig(data);
      setHasChanges(false);
      setToast({ message: 'Configuration updated successfully', type: 'success' });
    },
    onError: (error) => {
      const message = error instanceof SensorAPIError ? error.message : 'Failed to update configuration';
      setToast({ message, type: 'error' });
    },
  });

  // Reset configuration mutation
  const resetMutation = useMutation({
    mutationFn: async () => {
      const response = await sensorApi.resetConfig();
      return response.data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(['sensorConfig'], data);
      setLocalConfig(data);
      setHasChanges(false);
      setToast({ message: 'Configuration reset to defaults', type: 'success' });
    },
    onError: (error) => {
      const message = error instanceof SensorAPIError ? error.message : 'Failed to reset configuration';
      setToast({ message, type: 'error' });
    },
  });

  // Toggle pause/resume mutation
  const togglePauseMutation = useMutation({
    mutationFn: async (paused: boolean) => {
      const response = paused ? await sensorApi.pause() : await sensorApi.resume();
      return response.data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(['sensorConfig'], data);
      setLocalConfig(data);
      setToast({
        message: data.paused ? 'Sensor paused' : 'Sensor resumed',
        type: 'success'
      });
    },
    onError: (error) => {
      const message = error instanceof SensorAPIError ? error.message : 'Failed to toggle pause state';
      setToast({ message, type: 'error' });
    },
  });

  // Clear all data mutation
  const clearAllMutation = useMutation({
    mutationFn: async () => {
      const response = await clearApi.clearAll();
      return response.data;
    },
    onSuccess: (data) => {
      // Invalidate relevant queries to refresh the UI
      queryClient.invalidateQueries({ queryKey: ['tracks'] });
      queryClient.invalidateQueries({ queryKey: ['proposals'] });
      queryClient.invalidateQueries({ queryKey: ['decisions'] });
      queryClient.invalidateQueries({ queryKey: ['effects'] });
      const totalDeleted = data.deleted.tracks + data.deleted.proposals + data.deleted.decisions + data.deleted.effects + data.deleted.detections;
      setToast({
        message: `Cleared ${totalDeleted} records`,
        type: 'success'
      });
    },
    onError: (error) => {
      const message = error instanceof APIClientError ? error.message : 'Failed to clear data';
      setToast({ message, type: 'error' });
    },
  });

  // Handlers
  const handleEmissionIntervalChange = useCallback((value: number) => {
    if (localConfig) {
      setLocalConfig({ ...localConfig, emission_interval_ms: value });
      setHasChanges(true);
    }
  }, [localConfig]);

  const handleTrackCountChange = useCallback((value: number) => {
    if (localConfig) {
      setLocalConfig({ ...localConfig, track_count: value });
      setHasChanges(true);
    }
  }, [localConfig]);

  const handleTypeWeightChange = useCallback((key: keyof TrackTypeWeights, value: number) => {
    if (localConfig) {
      const currentWeights = localConfig.type_weights || DEFAULT_TYPE_WEIGHTS;
      setLocalConfig({
        ...localConfig,
        type_weights: { ...currentWeights, [key]: value },
      });
      setHasChanges(true);
    }
  }, [localConfig]);

  const handleClassificationWeightChange = useCallback((key: keyof ClassificationWeights, value: number) => {
    if (localConfig) {
      const currentWeights = localConfig.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS;
      setLocalConfig({
        ...localConfig,
        classification_weights: { ...currentWeights, [key]: value },
      });
      setHasChanges(true);
    }
  }, [localConfig]);

  const handleApplyChanges = useCallback(() => {
    if (localConfig && hasChanges) {
      updateMutation.mutate({
        emission_interval_ms: localConfig.emission_interval_ms,
        track_count: localConfig.track_count,
        type_weights: localConfig.type_weights,
        classification_weights: localConfig.classification_weights,
      });
    }
  }, [localConfig, hasChanges, updateMutation]);

  const handleTogglePause = useCallback(() => {
    if (localConfig) {
      togglePauseMutation.mutate(!localConfig.paused);
    }
  }, [localConfig, togglePauseMutation]);

  const handleReset = useCallback(() => {
    resetMutation.mutate();
  }, [resetMutation]);

  const handleDiscardChanges = useCallback(() => {
    if (configData) {
      setLocalConfig(configData);
      setHasChanges(false);
    }
  }, [configData]);

  const handleClearAll = useCallback(() => {
    if (!localConfig?.paused) {
      return; // Should not be callable when sensors are running
    }
    const confirmed = window.confirm(
      'Are you sure you want to clear all data? This will permanently delete all tracks, proposals, decisions, and effects.'
    );
    if (confirmed) {
      clearAllMutation.mutate();
    }
  }, [localConfig, clearAllMutation]);

  // Format emission interval for display
  const formatInterval = (ms: number): string => {
    if (ms >= 1000) {
      return (ms / 1000).toFixed(1);
    }
    return ms.toString();
  };

  const getIntervalUnit = (ms: number): string => {
    return ms >= 1000 ? 's' : 'ms';
  };

  // Loading state
  if (isLoading && !localConfig) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
          <span className="text-gray-400 text-sm">Loading sensor configuration...</span>
        </div>
      </div>
    );
  }

  // Error state
  if (error && !localConfig) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <svg
            className="mx-auto h-12 w-12 text-red-500"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-gray-300">Failed to load sensor configuration</h3>
          <p className="mt-1 text-sm text-gray-500">
            {error instanceof SensorAPIError ? error.message : 'Unable to connect to sensor'}
          </p>
          <button
            onClick={() => refetch()}
            className="mt-4 px-4 py-2 bg-green-600 hover:bg-green-700 text-white text-sm font-medium rounded-lg transition-colors"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  const config = localConfig || configData;
  if (!config) return null;

  const isMutating = updateMutation.isPending || resetMutation.isPending || togglePauseMutation.isPending || clearAllMutation.isPending;
  const canClear = config.paused && !isMutating;

  return (
    <div className="space-y-6">
      {/* Header Card */}
      <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-xl font-bold text-gray-100">Sensor Simulator Control Panel</h2>
            <p className="text-sm text-gray-400 mt-1">
              Configure the synthetic sensor simulation parameters
            </p>
          </div>
          <StatusIndicator paused={config.paused} isLoading={isMutating} />
        </div>
      </div>

      {/* Current Values Card */}
      <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
        <h3 className="text-sm font-medium text-gray-300 uppercase tracking-wide mb-4">
          Current Values
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div className="bg-gray-900 rounded-lg p-4 border border-gray-700">
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">Emission Interval</p>
            <p className="mt-1 text-2xl font-bold text-green-400 font-mono">
              {formatInterval(config.emission_interval_ms)}
              <span className="text-sm text-gray-500 ml-1">{getIntervalUnit(config.emission_interval_ms)}</span>
            </p>
          </div>
          <div className="bg-gray-900 rounded-lg p-4 border border-gray-700">
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">Track Count</p>
            <p className="mt-1 text-2xl font-bold text-blue-400 font-mono">
              {config.track_count}
              <span className="text-sm text-gray-500 ml-1">tracks</span>
            </p>
          </div>
          <div className="bg-gray-900 rounded-lg p-4 border border-gray-700">
            <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">Status</p>
            <p className={clsx(
              'mt-1 text-2xl font-bold font-mono',
              config.paused ? 'text-yellow-400' : 'text-green-400'
            )}>
              {config.paused ? 'PAUSED' : 'ACTIVE'}
            </p>
          </div>
        </div>
      </div>

      {/* Configuration Card */}
      <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
        <h3 className="text-sm font-medium text-gray-300 uppercase tracking-wide mb-6">
          Configuration
        </h3>

        <div className="space-y-8">
          {/* Emission Interval Slider */}
          <Slider
            label="Emission Interval"
            value={config.emission_interval_ms}
            min={100}
            max={10000}
            step={100}
            unit={getIntervalUnit(config.emission_interval_ms)}
            disabled={isMutating}
            formatValue={formatInterval}
            onChange={handleEmissionIntervalChange}
          />

          {/* Track Count Input */}
          <NumberInput
            label="Track Count"
            value={config.track_count}
            min={1}
            max={100}
            disabled={isMutating}
            onChange={handleTrackCountChange}
          />

          {/* Track Type Distribution */}
          <CollapsibleSection
            title="Track Type Distribution"
            isOpen={typeWeightsOpen}
            onToggle={() => setTypeWeightsOpen(!typeWeightsOpen)}
          >
            <div className="space-y-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-gray-500">Adjust weights to control track type distribution</span>
                <span className={clsx(
                  'text-xs font-mono',
                  (() => {
                    const weights = config.type_weights || DEFAULT_TYPE_WEIGHTS;
                    const sum = Object.values(weights).reduce((a, b) => a + b, 0);
                    return sum === 100 ? 'text-green-400' : 'text-yellow-400';
                  })()
                )}>
                  Total: {Object.values(config.type_weights || DEFAULT_TYPE_WEIGHTS).reduce((a, b) => a + b, 0)}%
                </span>
              </div>
              <WeightSlider
                label="Aircraft"
                value={(config.type_weights || DEFAULT_TYPE_WEIGHTS).aircraft}
                disabled={isMutating}
                color="accent-blue-500"
                onChange={(v) => handleTypeWeightChange('aircraft', v)}
              />
              <WeightSlider
                label="Vessel"
                value={(config.type_weights || DEFAULT_TYPE_WEIGHTS).vessel}
                disabled={isMutating}
                color="accent-cyan-500"
                onChange={(v) => handleTypeWeightChange('vessel', v)}
              />
              <WeightSlider
                label="Ground"
                value={(config.type_weights || DEFAULT_TYPE_WEIGHTS).ground}
                disabled={isMutating}
                color="accent-amber-500"
                onChange={(v) => handleTypeWeightChange('ground', v)}
              />
              <WeightSlider
                label="Missile"
                value={(config.type_weights || DEFAULT_TYPE_WEIGHTS).missile}
                disabled={isMutating}
                color="accent-red-500"
                onChange={(v) => handleTypeWeightChange('missile', v)}
              />
              <WeightSlider
                label="Unknown"
                value={(config.type_weights || DEFAULT_TYPE_WEIGHTS).unknown}
                disabled={isMutating}
                color="accent-gray-500"
                onChange={(v) => handleTypeWeightChange('unknown', v)}
              />
            </div>
          </CollapsibleSection>

          {/* Classification Distribution */}
          <CollapsibleSection
            title="Classification Distribution"
            isOpen={classificationWeightsOpen}
            onToggle={() => setClassificationWeightsOpen(!classificationWeightsOpen)}
          >
            <div className="space-y-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-gray-500">Adjust weights to control classification distribution</span>
                <span className={clsx(
                  'text-xs font-mono',
                  (() => {
                    const weights = config.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS;
                    const sum = Object.values(weights).reduce((a, b) => a + b, 0);
                    return sum === 100 ? 'text-green-400' : 'text-yellow-400';
                  })()
                )}>
                  Total: {Object.values(config.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS).reduce((a, b) => a + b, 0)}%
                </span>
              </div>
              <WeightSlider
                label="Friendly"
                value={(config.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS).friendly}
                disabled={isMutating}
                color="accent-green-500"
                onChange={(v) => handleClassificationWeightChange('friendly', v)}
              />
              <WeightSlider
                label="Hostile"
                value={(config.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS).hostile}
                disabled={isMutating}
                color="accent-red-500"
                onChange={(v) => handleClassificationWeightChange('hostile', v)}
              />
              <WeightSlider
                label="Neutral"
                value={(config.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS).neutral}
                disabled={isMutating}
                color="accent-blue-500"
                onChange={(v) => handleClassificationWeightChange('neutral', v)}
              />
              <WeightSlider
                label="Unknown"
                value={(config.classification_weights || DEFAULT_CLASSIFICATION_WEIGHTS).unknown}
                disabled={isMutating}
                color="accent-gray-500"
                onChange={(v) => handleClassificationWeightChange('unknown', v)}
              />
            </div>
          </CollapsibleSection>
        </div>

        {/* Apply/Discard Changes */}
        {hasChanges && (
          <div className="mt-6 pt-6 border-t border-gray-700">
            <div className="flex items-center justify-between">
              <span className="text-sm text-yellow-400">You have unsaved changes</span>
              <div className="flex gap-3">
                <button
                  onClick={handleDiscardChanges}
                  disabled={isMutating}
                  className={clsx(
                    'px-4 py-2 text-sm font-medium rounded-lg border border-gray-600',
                    'text-gray-300 hover:bg-gray-700 transition-colors',
                    isMutating && 'opacity-50 cursor-not-allowed'
                  )}
                >
                  Discard
                </button>
                <button
                  onClick={handleApplyChanges}
                  disabled={isMutating}
                  className={clsx(
                    'px-4 py-2 text-sm font-medium rounded-lg',
                    'bg-green-600 hover:bg-green-700 text-white transition-colors',
                    'flex items-center gap-2',
                    isMutating && 'opacity-50 cursor-not-allowed'
                  )}
                >
                  {updateMutation.isPending && (
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                  )}
                  Apply Changes
                </button>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Controls Card */}
      <div className="bg-gray-800 rounded-lg p-6 border border-gray-700">
        <h3 className="text-sm font-medium text-gray-300 uppercase tracking-wide mb-6">
          Controls
        </h3>

        <div className="flex flex-wrap gap-4">
          {/* Pause/Resume Button */}
          <button
            onClick={handleTogglePause}
            disabled={isMutating}
            className={clsx(
              'px-6 py-3 text-sm font-medium rounded-lg transition-colors flex items-center gap-2',
              config.paused
                ? 'bg-green-600 hover:bg-green-700 text-white'
                : 'bg-yellow-600 hover:bg-yellow-700 text-white',
              isMutating && 'opacity-50 cursor-not-allowed'
            )}
          >
            {togglePauseMutation.isPending ? (
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
            ) : config.paused ? (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            ) : (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            )}
            {config.paused ? 'Resume' : 'Pause'}
          </button>

          {/* Reset Button */}
          <button
            onClick={handleReset}
            disabled={isMutating}
            className={clsx(
              'px-6 py-3 text-sm font-medium rounded-lg border border-gray-600',
              'text-gray-300 hover:bg-gray-700 transition-colors flex items-center gap-2',
              isMutating && 'opacity-50 cursor-not-allowed'
            )}
          >
            {resetMutation.isPending ? (
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-300" />
            ) : (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            )}
            Reset to Defaults
          </button>

          {/* Clear All Button */}
          <button
            onClick={handleClearAll}
            disabled={!canClear}
            title={!config.paused ? 'Pause sensors first' : undefined}
            className={clsx(
              'px-6 py-3 text-sm font-medium rounded-lg transition-colors flex items-center gap-2',
              canClear
                ? 'bg-red-600 hover:bg-red-700 text-white border border-red-600'
                : 'bg-gray-700 text-gray-500 border border-gray-600 cursor-not-allowed'
            )}
          >
            {clearAllMutation.isPending ? (
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
            ) : (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            )}
            Clear All Data
          </button>
        </div>
      </div>

      {/* Toast Notification */}
      {toast && (
        <Toast
          message={toast.message}
          type={toast.type}
          onClose={() => setToast(null)}
        />
      )}
    </div>
  );
}

export default SensorControlPage;
