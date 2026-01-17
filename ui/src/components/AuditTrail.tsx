import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { format, formatDistanceToNow } from 'date-fns';
import clsx from 'clsx';
import { api } from '../api/client';
import type { AuditEntry, ActionType } from '../types';

interface AuditTrailProps {
  className?: string;
}

// Status colors and icons
const statusConfig: Record<string, { color: string; bgColor: string; icon: string }> = {
  proposed: {
    color: 'text-blue-400',
    bgColor: 'bg-blue-900/30',
    icon: 'M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
  },
  approved: {
    color: 'text-green-400',
    bgColor: 'bg-green-900/30',
    icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
  },
  denied: {
    color: 'text-red-400',
    bgColor: 'bg-red-900/30',
    icon: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z',
  },
  executed: {
    color: 'text-purple-400',
    bgColor: 'bg-purple-900/30',
    icon: 'M13 10V3L4 14h7v7l9-11h-7z',
  },
  failed: {
    color: 'text-orange-400',
    bgColor: 'bg-orange-900/30',
    icon: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z',
  },
};

// Action type colors
const actionTypeColors: Record<ActionType, string> = {
  engage: 'text-red-400',
  intercept: 'text-orange-400',
  track: 'text-blue-400',
  identify: 'text-cyan-400',
  monitor: 'text-green-400',
  ignore: 'text-gray-400',
};

// Filter options
const actionTypes: ActionType[] = ['engage', 'intercept', 'track', 'identify', 'monitor', 'ignore'];
const statusOptions = ['proposed', 'approved', 'denied', 'executed', 'failed'];

export function AuditTrail({ className }: AuditTrailProps) {
  const [selectedActionType, setSelectedActionType] = useState<string>('');
  const [selectedStatus, setSelectedStatus] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState('');
  const [expandedEntry, setExpandedEntry] = useState<string | null>(null);

  // Fetch audit entries
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['audit', selectedActionType, selectedStatus],
    queryFn: async () => {
      const response = await api.audit.getEntries({
        limit: 100,
        action_type: selectedActionType || undefined,
      });
      return response.data;
    },
    refetchInterval: 10000,
    staleTime: 5000,
  });

  // Filter entries locally
  const filteredEntries = useMemo(() => {
    if (!data) return [];

    let entries = data;

    // Filter by status
    if (selectedStatus) {
      entries = entries.filter((e) => e.status === selectedStatus);
    }

    // Filter by search query
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      entries = entries.filter(
        (e) =>
          e.track_id.toLowerCase().includes(query) ||
          e.action_type.toLowerCase().includes(query) ||
          (e.user_id && e.user_id.toLowerCase().includes(query)) ||
          e.details.toLowerCase().includes(query) ||
          (e.reason && e.reason.toLowerCase().includes(query))
      );
    }

    return entries;
  }, [data, selectedStatus, searchQuery]);

  // Group entries by correlation chain
  const groupedEntries = useMemo(() => {
    const groups = new Map<string, AuditEntry[]>();

    filteredEntries.forEach((entry) => {
      const key = entry.proposal_id || entry.id;
      if (!groups.has(key)) {
        groups.set(key, []);
      }
      groups.get(key)!.push(entry);
    });

    // Sort each group by timestamp
    groups.forEach((entries) => {
      entries.sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
    });

    return Array.from(groups.entries()).sort((a, b) => {
      const aTime = new Date(a[1][a[1].length - 1].timestamp).getTime();
      const bTime = new Date(b[1][b[1].length - 1].timestamp).getTime();
      return bTime - aTime;
    });
  }, [filteredEntries]);

  if (isLoading && !data) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
          <span className="text-gray-400 text-sm">Loading audit trail...</span>
        </div>
      </div>
    );
  }

  return (
    <div className={clsx('space-y-4', className)}>
      {/* Filters */}
      <div className="flex flex-wrap items-center gap-4 bg-gray-800 rounded-lg p-4 border border-gray-700">
        {/* Search */}
        <div className="flex-1 min-w-[200px]">
          <div className="relative">
            <svg
              className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-500"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
            <input
              type="text"
              placeholder="Search track ID, user, reason, or details..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full pl-10 pr-4 py-2 bg-gray-900 border border-gray-700 rounded text-gray-200 placeholder-gray-500 text-sm focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500"
            />
          </div>
        </div>

        {/* Action Type Filter */}
        <select
          value={selectedActionType}
          onChange={(e) => setSelectedActionType(e.target.value)}
          className="px-3 py-2 bg-gray-900 border border-gray-700 rounded text-gray-200 text-sm focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500"
        >
          <option value="">All Actions</option>
          {actionTypes.map((type) => (
            <option key={type} value={type}>
              {type.charAt(0).toUpperCase() + type.slice(1)}
            </option>
          ))}
        </select>

        {/* Status Filter */}
        <select
          value={selectedStatus}
          onChange={(e) => setSelectedStatus(e.target.value)}
          className="px-3 py-2 bg-gray-900 border border-gray-700 rounded text-gray-200 text-sm focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500"
        >
          <option value="">All Statuses</option>
          {statusOptions.map((status) => (
            <option key={status} value={status}>
              {status.charAt(0).toUpperCase() + status.slice(1)}
            </option>
          ))}
        </select>

        {/* Refresh button */}
        <button
          onClick={() => refetch()}
          className="px-3 py-2 bg-gray-700 hover:bg-gray-600 text-gray-300 rounded text-sm transition-colors"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
            />
          </svg>
        </button>
      </div>

      {/* Results count */}
      <div className="flex items-center justify-between text-sm">
        <span className="text-gray-500">
          Showing <span className="text-gray-300">{filteredEntries.length}</span> entries
          {groupedEntries.length !== filteredEntries.length && (
            <span> in <span className="text-gray-300">{groupedEntries.length}</span> chains</span>
          )}
        </span>
      </div>

      {/* Audit entries */}
      {filteredEntries.length === 0 ? (
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
                d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
              />
            </svg>
            <h3 className="mt-2 text-sm font-medium text-gray-400">No audit entries</h3>
            <p className="mt-1 text-sm text-gray-500">
              {searchQuery || selectedActionType || selectedStatus
                ? 'No entries match your filters.'
                : 'No audit events recorded yet.'}
            </p>
          </div>
        </div>
      ) : (
        <div className="space-y-2">
          {groupedEntries.map(([chainId, entries]) => {
            const isExpanded = expandedEntry === chainId;
            const latestEntry = entries[entries.length - 1];
            const config = statusConfig[latestEntry.status];

            return (
              <div
                key={chainId}
                className="bg-gray-900 rounded-lg border border-gray-700 overflow-hidden"
              >
                {/* Main entry row */}
                <button
                  onClick={() => setExpandedEntry(isExpanded ? null : chainId)}
                  className="w-full px-4 py-3 flex items-center gap-4 hover:bg-gray-800/50 transition-colors text-left"
                >
                  {/* Status icon */}
                  <div className={clsx('p-2 rounded-full', config.bgColor)}>
                    <svg
                      className={clsx('w-5 h-5', config.color)}
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d={config.icon}
                      />
                    </svg>
                  </div>

                  {/* Entry info */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span
                        className={clsx(
                          'font-medium text-sm uppercase',
                          actionTypeColors[latestEntry.action_type as ActionType]
                        )}
                      >
                        {latestEntry.action_type}
                      </span>
                      <span className={clsx('text-xs px-2 py-0.5 rounded', config.bgColor, config.color)}>
                        {latestEntry.status}
                      </span>
                      {entries.length > 1 && (
                        <span className="text-xs text-gray-500">
                          ({entries.length} events)
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-3 mt-1 text-xs text-gray-500">
                      <span>
                        Track: <span className="text-gray-400 font-mono">{latestEntry.track_id}</span>
                      </span>
                      {latestEntry.user_id && (
                        <span>
                          User: <span className="text-gray-400">{latestEntry.user_id}</span>
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Timestamp */}
                  <div className="text-right text-xs text-gray-500">
                    <div>{format(new Date(latestEntry.timestamp), 'MMM d, HH:mm:ss')}</div>
                    <div>{formatDistanceToNow(new Date(latestEntry.timestamp), { addSuffix: true })}</div>
                  </div>

                  {/* Expand icon */}
                  <svg
                    className={clsx(
                      'w-5 h-5 text-gray-500 transition-transform',
                      isExpanded && 'rotate-180'
                    )}
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
                </button>

                {/* Expanded details */}
                {isExpanded && (
                  <div className="border-t border-gray-700 px-4 py-3 bg-gray-800/30">
                    <div className="space-y-4">
                      {/* Timeline of events */}
                      <div className="relative pl-6">
                        {entries.map((entry, index) => {
                          const entryConfig = statusConfig[entry.status];
                          const isLast = index === entries.length - 1;

                          return (
                            <div key={entry.id} className="relative pb-4">
                              {/* Timeline line */}
                              {!isLast && (
                                <div className="absolute left-[-18px] top-6 bottom-0 w-0.5 bg-gray-700" />
                              )}

                              {/* Timeline dot */}
                              <div
                                className={clsx(
                                  'absolute left-[-22px] top-1 w-2 h-2 rounded-full',
                                  entryConfig.color.replace('text-', 'bg-')
                                )}
                              />

                              <div className="flex items-start justify-between">
                                <div>
                                  <div className="flex items-center gap-2">
                                    <span
                                      className={clsx(
                                        'text-xs px-2 py-0.5 rounded',
                                        entryConfig.bgColor,
                                        entryConfig.color
                                      )}
                                    >
                                      {entry.status}
                                    </span>
                                  </div>
                                  {entry.details && (
                                    <p className="mt-1 text-sm text-gray-400">{entry.details}</p>
                                  )}
                                  {entry.reason && (
                                    <p className="mt-1 text-sm text-gray-300">
                                      <span className="text-gray-500 font-medium">Reason: </span>
                                      {entry.reason}
                                    </p>
                                  )}
                                  {entry.user_id && (
                                    <p className="mt-1 text-xs text-gray-500">
                                      By: {entry.user_id}
                                    </p>
                                  )}
                                </div>
                                <span className="text-xs text-gray-500 whitespace-nowrap ml-4">
                                  {format(new Date(entry.timestamp), 'HH:mm:ss.SSS')}
                                </span>
                              </div>
                            </div>
                          );
                        })}
                      </div>

                      {/* IDs section */}
                      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 pt-4 border-t border-gray-700">
                        <div>
                          <label className="block text-xs font-medium text-gray-500 uppercase">
                            Proposal ID
                          </label>
                          <p className="mt-1 text-xs text-gray-400 font-mono truncate">
                            {latestEntry.proposal_id || '-'}
                          </p>
                        </div>
                        <div>
                          <label className="block text-xs font-medium text-gray-500 uppercase">
                            Decision ID
                          </label>
                          <p className="mt-1 text-xs text-gray-400 font-mono truncate">
                            {latestEntry.decision_id || '-'}
                          </p>
                        </div>
                        <div>
                          <label className="block text-xs font-medium text-gray-500 uppercase">
                            Effect ID
                          </label>
                          <p className="mt-1 text-xs text-gray-400 font-mono truncate">
                            {latestEntry.effect_id || '-'}
                          </p>
                        </div>
                        <div>
                          <label className="block text-xs font-medium text-gray-500 uppercase">
                            Track ID
                          </label>
                          <p className="mt-1 text-xs text-gray-400 font-mono truncate">
                            {latestEntry.track_id}
                          </p>
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export default AuditTrail;
