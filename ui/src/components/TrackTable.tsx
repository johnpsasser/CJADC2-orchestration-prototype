import { useMemo, useCallback } from 'react';
import { formatDistanceToNow } from 'date-fns';
import clsx from 'clsx';
import type { CorrelatedTrack, ThreatLevel, SortConfig } from '../types';

interface TrackTableProps {
  tracks: CorrelatedTrack[];
  selectedTrackId: string | null;
  sortConfig: SortConfig;
  onSelectTrack: (trackId: string | null) => void;
  onSort: (key: string) => void;
  isLoading?: boolean;
}

// Threat level color mapping
const threatLevelColors: Record<ThreatLevel, { bg: string; text: string; border: string }> = {
  critical: {
    bg: 'bg-red-900/30',
    text: 'text-red-400',
    border: 'border-red-500/50',
  },
  high: {
    bg: 'bg-orange-900/30',
    text: 'text-orange-400',
    border: 'border-orange-500/50',
  },
  medium: {
    bg: 'bg-yellow-900/30',
    text: 'text-yellow-400',
    border: 'border-yellow-500/50',
  },
  low: {
    bg: 'bg-green-900/30',
    text: 'text-green-400',
    border: 'border-green-500/50',
  },
  unknown: {
    bg: 'bg-gray-900/30',
    text: 'text-gray-400',
    border: 'border-gray-500/50',
  },
};

// Default colors for unknown threat levels
const defaultThreatColors = threatLevelColors.unknown;

// Classification badge colors
const classificationColors: Record<string, string> = {
  hostile: 'bg-red-600 text-white',
  friendly: 'bg-blue-600 text-white',
  neutral: 'bg-gray-500 text-white',
  unknown: 'bg-gray-700 text-gray-300',
};

// Column definitions
const columns = [
  { key: 'track_id', label: 'Track ID', sortable: true },
  { key: 'classification', label: 'Classification', sortable: true },
  { key: 'type', label: 'Type', sortable: true },
  { key: 'threat_level', label: 'Threat', sortable: true },
  { key: 'position', label: 'Position', sortable: false },
  { key: 'velocity', label: 'Velocity', sortable: false },
  { key: 'confidence', label: 'Confidence', sortable: true },
  { key: 'last_updated', label: 'Last Updated', sortable: true },
];

export function TrackTable({
  tracks,
  selectedTrackId,
  sortConfig,
  onSelectTrack,
  onSort,
  isLoading = false,
}: TrackTableProps) {
  // Format position for display
  const formatPosition = useCallback((pos: CorrelatedTrack['position'] | undefined) => {
    if (!pos || typeof pos.lat !== 'number' || typeof pos.lon !== 'number') {
      return 'Unknown';
    }
    const latDir = pos.lat >= 0 ? 'N' : 'S';
    const lonDir = pos.lon >= 0 ? 'E' : 'W';
    return `${Math.abs(pos.lat).toFixed(4)}${latDir} ${Math.abs(pos.lon).toFixed(4)}${lonDir}`;
  }, []);

  // Format velocity for display
  const formatVelocity = useCallback((vel: CorrelatedTrack['velocity'] | undefined) => {
    if (!vel || typeof vel.speed !== 'number' || typeof vel.heading !== 'number') {
      return 'Unknown';
    }
    return `${vel.speed.toFixed(0)} m/s @ ${vel.heading.toFixed(0)}deg`;
  }, []);

  // Format confidence as percentage
  const formatConfidence = useCallback((confidence: number | undefined) => {
    if (typeof confidence !== 'number') {
      return 'Unknown';
    }
    return `${(confidence * 100).toFixed(0)}%`;
  }, []);

  // Format time as relative
  const formatTime = useCallback((timestamp: string | undefined) => {
    if (!timestamp) {
      return 'Unknown';
    }
    try {
      const date = new Date(timestamp);
      // Check if the date is valid
      if (isNaN(date.getTime())) {
        return 'Unknown';
      }
      return formatDistanceToNow(date, { addSuffix: true });
    } catch {
      return 'Unknown';
    }
  }, []);

  // Sort icon component
  const SortIcon = useMemo(
    () =>
      ({ columnKey }: { columnKey: string }) => {
        if (sortConfig.key !== columnKey) {
          return (
            <svg
              className="w-4 h-4 text-gray-600"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"
              />
            </svg>
          );
        }
        return sortConfig.direction === 'asc' ? (
          <svg
            className="w-4 h-4 text-green-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M5 15l7-7 7 7"
            />
          </svg>
        ) : (
          <svg
            className="w-4 h-4 text-green-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M19 9l-7 7-7-7"
            />
          </svg>
        );
      },
    [sortConfig]
  );

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
          <span className="text-gray-400 text-sm">Loading tracks...</span>
        </div>
      </div>
    );
  }

  if (tracks.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
        <div className="text-center">
          <svg
            className="mx-auto h-12 w-12 text-gray-600"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1}
              d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"
            />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-gray-400">No tracks</h3>
          <p className="mt-1 text-sm text-gray-500">
            No active tracks detected in the system.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="overflow-hidden bg-gray-900 rounded-lg border border-gray-700">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-gray-700">
          <thead className="bg-gray-800">
            <tr>
              {columns.map((column) => (
                <th
                  key={column.key}
                  scope="col"
                  className={clsx(
                    'px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider',
                    column.sortable && 'cursor-pointer hover:bg-gray-700 select-none'
                  )}
                  onClick={() => column.sortable && onSort(column.key)}
                >
                  <div className="flex items-center gap-1">
                    {column.label}
                    {column.sortable && <SortIcon columnKey={column.key} />}
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800">
            {tracks.map((track) => {
              const colors = threatLevelColors[track.threat_level || 'unknown'] || defaultThreatColors;
              const isSelected = selectedTrackId === track.track_id;

              return (
                <tr
                  key={track.track_id}
                  className={clsx(
                    'cursor-pointer transition-colors',
                    isSelected
                      ? `${colors.bg} ${colors.border} border-l-4`
                      : 'hover:bg-gray-800/50 border-l-4 border-transparent'
                  )}
                  onClick={() =>
                    onSelectTrack(isSelected ? null : track.track_id)
                  }
                >
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span className="font-mono text-sm text-gray-300">
                      {track.track_id}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span
                      className={clsx(
                        'px-2 py-1 text-xs font-medium rounded-full',
                        classificationColors[track.classification] || classificationColors.unknown
                      )}
                    >
                      {track.classification.toUpperCase()}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span className="text-sm text-gray-300 capitalize">
                      {track.type}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span
                      className={clsx(
                        'px-2 py-1 text-xs font-medium rounded',
                        colors.bg,
                        colors.text
                      )}
                    >
                      {(track.threat_level || 'unknown').toUpperCase()}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span className="font-mono text-xs text-gray-400">
                      {formatPosition(track.position)}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span className="font-mono text-xs text-gray-400">
                      {formatVelocity(track.velocity)}
                    </span>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <div className="flex items-center gap-2">
                      <div className="w-16 bg-gray-700 rounded-full h-2">
                        <div
                          className={clsx(
                            'h-2 rounded-full transition-all',
                            (track.confidence ?? 0) >= 0.8
                              ? 'bg-green-500'
                              : (track.confidence ?? 0) >= 0.5
                              ? 'bg-yellow-500'
                              : 'bg-red-500'
                          )}
                          style={{ width: `${(track.confidence ?? 0) * 100}%` }}
                        />
                      </div>
                      <span className="text-xs text-gray-400">
                        {formatConfidence(track.confidence)}
                      </span>
                    </div>
                  </td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <span className="text-xs text-gray-400">
                      {formatTime(track.last_updated || track.window_end)}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Footer with count */}
      <div className="bg-gray-800 px-4 py-2 border-t border-gray-700">
        <p className="text-xs text-gray-500">
          Showing <span className="font-medium text-gray-400">{tracks.length}</span> tracks
        </p>
      </div>
    </div>
  );
}

// Track detail panel component
interface TrackDetailProps {
  track: CorrelatedTrack;
  onClose: () => void;
}

export function TrackDetail({ track, onClose }: TrackDetailProps) {
  const colors = threatLevelColors[track.threat_level || 'unknown'] || defaultThreatColors;

  return (
    <div className={clsx('bg-gray-900 rounded-lg border', colors.border)}>
      <div className={clsx('px-4 py-3 border-b', colors.border, colors.bg)}>
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-medium text-gray-200">Track Details</h3>
            <p className="text-xs text-gray-400 font-mono">{track.track_id}</p>
          </div>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-gray-300"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </div>

      <div className="p-4 space-y-4">
        {/* Classification & Threat */}
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-xs font-medium text-gray-500 uppercase">
              Classification
            </label>
            <span
              className={clsx(
                'inline-block mt-1 px-2 py-1 text-xs font-medium rounded-full',
                classificationColors[track.classification]
              )}
            >
              {track.classification.toUpperCase()}
            </span>
          </div>
          <div>
            <label className="block text-xs font-medium text-gray-500 uppercase">
              Threat Level
            </label>
            <span
              className={clsx(
                'inline-block mt-1 px-2 py-1 text-xs font-medium rounded',
                colors.bg,
                colors.text
              )}
            >
              {(track.threat_level || 'unknown').toUpperCase()}
            </span>
          </div>
        </div>

        {/* Type */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">Type</label>
          <p className="mt-1 text-sm text-gray-300 capitalize">{track.type}</p>
        </div>

        {/* Position */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">Position</label>
          <div className="mt-1 grid grid-cols-3 gap-2 text-xs">
            <div>
              <span className="text-gray-500">LAT:</span>
              <span className="ml-1 text-gray-300 font-mono">{track.position?.lat?.toFixed(6) ?? 'N/A'}</span>
            </div>
            <div>
              <span className="text-gray-500">LON:</span>
              <span className="ml-1 text-gray-300 font-mono">{track.position?.lon?.toFixed(6) ?? 'N/A'}</span>
            </div>
            <div>
              <span className="text-gray-500">ALT:</span>
              <span className="ml-1 text-gray-300 font-mono">{track.position?.alt?.toFixed(0) ?? 'N/A'}m</span>
            </div>
          </div>
        </div>

        {/* Velocity */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">Velocity</label>
          <div className="mt-1 grid grid-cols-2 gap-2 text-xs">
            <div>
              <span className="text-gray-500">Speed:</span>
              <span className="ml-1 text-gray-300 font-mono">{track.velocity?.speed?.toFixed(1) ?? 'N/A'} m/s</span>
            </div>
            <div>
              <span className="text-gray-500">Heading:</span>
              <span className="ml-1 text-gray-300 font-mono">{track.velocity?.heading?.toFixed(1) ?? 'N/A'} deg</span>
            </div>
          </div>
        </div>

        {/* Confidence */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">Confidence</label>
          <div className="mt-1 flex items-center gap-2">
            <div className="flex-1 bg-gray-700 rounded-full h-2">
              <div
                className={clsx(
                  'h-2 rounded-full',
                  (track.confidence ?? 0) >= 0.8
                    ? 'bg-green-500'
                    : (track.confidence ?? 0) >= 0.5
                    ? 'bg-yellow-500'
                    : 'bg-red-500'
                )}
                style={{ width: `${(track.confidence ?? 0) * 100}%` }}
              />
            </div>
            <span className="text-sm text-gray-300">{((track.confidence ?? 0) * 100).toFixed(0)}%</span>
          </div>
        </div>

        {/* Sources */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">Sources</label>
          <div className="mt-1 flex flex-wrap gap-1">
            {(track.sources ?? []).map((source) => (
              <span
                key={source}
                className="px-2 py-0.5 text-xs bg-gray-800 text-gray-400 rounded"
              >
                {source}
              </span>
            ))}
          </div>
        </div>

        {/* Detection Count */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">
            Detection Count
          </label>
          <p className="mt-1 text-sm text-gray-300">{track.detection_count ?? 0}</p>
        </div>

        {/* Correlation Info */}
        <div>
          <label className="block text-xs font-medium text-gray-500 uppercase">
            Merged From
          </label>
          <p className="mt-1 text-xs text-gray-400 font-mono">
            {(track.merged_from ?? []).join(', ') || 'N/A'}
          </p>
        </div>
      </div>
    </div>
  );
}

export default TrackTable;
