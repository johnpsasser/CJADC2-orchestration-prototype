import { useState, useEffect, useMemo } from 'react';
import { differenceInSeconds } from 'date-fns';
import clsx from 'clsx';
import type { ActionProposal, ThreatLevel, ActionType } from '../types';

interface ProposalQueueProps {
  proposals: ActionProposal[];
  onApprove: (proposalId: string) => void;
  onDeny: (proposalId: string) => void;
  isLoading?: boolean;
}

// Priority color mapping
const priorityColors: Record<string, { bg: string; text: string; border: string }> = {
  critical: { bg: 'bg-red-900/40', text: 'text-red-300', border: 'border-red-500/50' },
  high: { bg: 'bg-orange-900/40', text: 'text-orange-300', border: 'border-orange-500/50' },
  medium: { bg: 'bg-sky-900/40', text: 'text-sky-300', border: 'border-sky-500/50' },
  normal: { bg: 'bg-gray-800', text: 'text-gray-300', border: 'border-gray-600' },
};

// Action type icons and colors
const actionTypeConfig: Record<ActionType, { icon: string; color: string }> = {
  engage: { icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-red-400' },
  intercept: { icon: 'M13 10V3L4 14h7v7l9-11h-7z', color: 'text-orange-400' },
  track: { icon: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z', color: 'text-blue-400' },
  identify: { icon: 'M10 6H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V8a2 2 0 00-2-2h-5m-4 0V5a2 2 0 114 0v1m-4 0a2 2 0 104 0m-5 8a2 2 0 100-4 2 2 0 000 4zm0 0c1.306 0 2.417.835 2.83 2M9 14a3.001 3.001 0 00-2.83 2M15 11h3m-3 4h2', color: 'text-cyan-400' },
  monitor: { icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z', color: 'text-green-400' },
  ignore: { icon: 'M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636', color: 'text-gray-400' },
};

// Threat level colors (matching TrackTable)
const threatLevelColors: Record<ThreatLevel, string> = {
  critical: 'text-red-400',
  high: 'text-orange-400',
  medium: 'text-yellow-400',
  low: 'text-green-400',
  unknown: 'text-gray-400',
};

// Get priority level from numeric priority
function getPriorityLevel(priority: number): string {
  if (priority >= 9) return 'critical';
  if (priority >= 7) return 'high';
  if (priority >= 4) return 'medium';
  return 'normal';
}

// Countdown timer component
function ExpirationCountdown({ expiresAt }: { expiresAt: string }) {
  const [secondsLeft, setSecondsLeft] = useState(() =>
    Math.max(0, differenceInSeconds(new Date(expiresAt), new Date()))
  );

  useEffect(() => {
    const interval = setInterval(() => {
      const remaining = differenceInSeconds(new Date(expiresAt), new Date());
      setSecondsLeft(Math.max(0, remaining));
    }, 1000);

    return () => clearInterval(interval);
  }, [expiresAt]);

  const minutes = Math.floor(secondsLeft / 60);
  const seconds = secondsLeft % 60;
  const isUrgent = secondsLeft <= 60;
  const isCritical = secondsLeft <= 30;

  if (secondsLeft <= 0) {
    return <span className="text-red-500 font-medium">EXPIRED</span>;
  }

  return (
    <div className="flex items-center gap-2">
      <div
        className={clsx(
          'relative w-20 h-2 bg-gray-700 rounded-full overflow-hidden',
          isCritical && 'animate-pulse'
        )}
      >
        <div
          className={clsx(
            'absolute inset-y-0 left-0 rounded-full transition-all duration-1000',
            isCritical ? 'bg-red-500' : isUrgent ? 'bg-orange-500' : 'bg-green-500'
          )}
          style={{
            width: `${Math.min(100, (secondsLeft / 300) * 100)}%`,
          }}
        />
      </div>
      <span
        className={clsx(
          'font-mono text-sm',
          isCritical ? 'text-red-400' : isUrgent ? 'text-orange-400' : 'text-gray-400'
        )}
      >
        {minutes}:{seconds.toString().padStart(2, '0')}
      </span>
    </div>
  );
}

// Single proposal card component
interface ProposalCardProps {
  proposal: ActionProposal;
  onApprove: () => void;
  onDeny: () => void;
}

function ProposalCard({ proposal, onApprove, onDeny }: ProposalCardProps) {
  const priorityLevel = getPriorityLevel(proposal.priority);
  const colors = priorityColors[priorityLevel];
  const actionConfig = actionTypeConfig[proposal.action_type];

  return (
    <div
      className={clsx(
        'rounded-lg border p-4 transition-all',
        colors.bg,
        colors.border,
        'hover:shadow-lg hover:shadow-black/20'
      )}
    >
      {/* Header */}
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <svg
            className={clsx('w-5 h-5', actionConfig.color)}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d={actionConfig.icon}
            />
          </svg>
          <span className={clsx('font-medium text-sm uppercase', actionConfig.color)}>
            {proposal.action_type}
          </span>
          <span
            className={clsx(
              'px-2 py-0.5 text-xs font-medium rounded',
              priorityLevel === 'critical'
                ? 'bg-red-500/30 text-red-300'
                : priorityLevel === 'high'
                ? 'bg-orange-500/30 text-orange-300'
                : priorityLevel === 'medium'
                ? 'bg-sky-500/30 text-sky-300'
                : 'bg-gray-600 text-gray-300'
            )}
          >
            P{proposal.priority}
          </span>
        </div>
        <ExpirationCountdown expiresAt={proposal.expires_at} />
      </div>

      {/* Track Info */}
      <div className="mb-3 space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Track:</span>
          <span className="font-mono text-sm text-gray-300">{proposal.track_id}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Type:</span>
          <span className="text-sm text-gray-300 capitalize">{proposal.track?.type || 'unknown'}</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Threat:</span>
          <span className={clsx('text-sm font-medium', threatLevelColors[proposal.threat_level])}>
            {proposal.threat_level.toUpperCase()}
          </span>
        </div>
      </div>

      {/* Rationale */}
      <div className="mb-3">
        <p className="text-sm text-gray-400 line-clamp-2">{proposal.rationale}</p>
      </div>

      {/* Policy Decision Summary */}
      {proposal.policy_decision && (
        <div className="mb-3 p-2 bg-gray-900/50 rounded">
          <div className="flex items-center gap-2 mb-1">
            <span
              className={clsx(
                'w-2 h-2 rounded-full',
                proposal.policy_decision.allowed ? 'bg-green-500' : 'bg-red-500'
              )}
            />
            <span className="text-xs text-gray-400">
              Policy: {proposal.policy_decision.allowed ? 'Allowed' : 'Denied'}
            </span>
          </div>
          {proposal.policy_decision.warnings && proposal.policy_decision.warnings.length > 0 && (
            <p className="text-xs text-yellow-400">
              Warning: {proposal.policy_decision.warnings[0]}
            </p>
          )}
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2">
        <button
          onClick={onApprove}
          className="flex-1 px-3 py-2 bg-green-600 hover:bg-green-500 text-white text-sm font-medium rounded transition-colors"
        >
          Approve
        </button>
        <button
          onClick={onDeny}
          className="flex-1 px-3 py-2 bg-red-600 hover:bg-red-500 text-white text-sm font-medium rounded transition-colors"
        >
          Deny
        </button>
      </div>
    </div>
  );
}

// List view component
interface ProposalListViewProps {
  proposals: ActionProposal[];
  onApprove: (proposalId: string) => void;
  onDeny: (proposalId: string) => void;
}

function ProposalListView({ proposals, onApprove, onDeny }: ProposalListViewProps) {
  return (
    <div className="bg-gray-900 border border-gray-700 rounded-lg overflow-hidden">
      <table className="w-full">
        <thead className="bg-gray-800/50 border-b border-gray-700">
          <tr>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
              Action Type
            </th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
              Track ID
            </th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
              Type
            </th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
              Threat Level
            </th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
              Priority
            </th>
            <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
              Time Remaining
            </th>
            <th className="px-4 py-3 text-right text-xs font-medium text-gray-400 uppercase tracking-wider">
              Actions
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-700">
          {proposals.map((proposal) => {
            const priorityLevel = getPriorityLevel(proposal.priority);
            const actionConfig = actionTypeConfig[proposal.action_type];

            return (
              <tr
                key={proposal.proposal_id}
                className="hover:bg-gray-800/50 transition-colors"
              >
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <svg
                      className={clsx('w-4 h-4', actionConfig.color)}
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d={actionConfig.icon}
                      />
                    </svg>
                    <span className={clsx('text-sm font-medium uppercase', actionConfig.color)}>
                      {proposal.action_type}
                    </span>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <span className="font-mono text-sm text-gray-300">{proposal.track_id}</span>
                </td>
                <td className="px-4 py-3">
                  <span className="text-sm text-gray-300 capitalize">{proposal.track?.type || 'unknown'}</span>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={clsx(
                      'inline-flex px-2 py-1 text-xs font-medium rounded',
                      proposal.threat_level === 'critical'
                        ? 'bg-red-900/40 text-red-300'
                        : proposal.threat_level === 'high'
                        ? 'bg-orange-900/40 text-orange-300'
                        : proposal.threat_level === 'medium'
                        ? 'bg-yellow-900/40 text-yellow-300'
                        : proposal.threat_level === 'low'
                        ? 'bg-green-900/40 text-green-300'
                        : 'bg-gray-700 text-gray-300'
                    )}
                  >
                    {proposal.threat_level.toUpperCase()}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={clsx(
                      'inline-flex px-2 py-1 text-xs font-medium rounded',
                      priorityLevel === 'critical'
                        ? 'bg-red-500/30 text-red-300'
                        : priorityLevel === 'high'
                        ? 'bg-orange-500/30 text-orange-300'
                        : priorityLevel === 'medium'
                        ? 'bg-sky-500/30 text-sky-300'
                        : 'bg-gray-600 text-gray-300'
                    )}
                  >
                    P{proposal.priority}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <ExpirationCountdown expiresAt={proposal.expires_at} />
                </td>
                <td className="px-4 py-3">
                  <div className="flex items-center justify-end gap-2">
                    <button
                      onClick={() => onApprove(proposal.proposal_id)}
                      className="px-2 py-1 bg-green-600 hover:bg-green-500 text-white text-xs font-medium rounded transition-colors"
                    >
                      Approve
                    </button>
                    <button
                      onClick={() => onDeny(proposal.proposal_id)}
                      className="px-2 py-1 bg-red-600 hover:bg-red-500 text-white text-xs font-medium rounded transition-colors"
                    >
                      Deny
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function ProposalQueue({
  proposals,
  onApprove,
  onDeny,
  isLoading = false,
}: ProposalQueueProps) {
  const [viewMode, setViewMode] = useState<'card' | 'list'>('card');

  // Group proposals by priority level
  const groupedProposals = useMemo(() => {
    const groups: Record<string, ActionProposal[]> = {
      critical: [],
      high: [],
      medium: [],
      normal: [],
    };

    proposals.forEach((p) => {
      const level = getPriorityLevel(p.priority);
      groups[level].push(p);
    });

    return groups;
  }, [proposals]);

  // Count urgent proposals (expiring within 60s)
  const urgentCount = useMemo(() => {
    const now = Date.now();
    const threshold = 60 * 1000;
    return proposals.filter((p) => {
      const expiresAt = new Date(p.expires_at).getTime();
      return expiresAt - now <= threshold && expiresAt > now;
    }).length;
  }, [proposals]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
          <span className="text-gray-400 text-sm">Loading proposals...</span>
        </div>
      </div>
    );
  }

  if (proposals.length === 0) {
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
              d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
            />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-gray-400">No pending proposals</h3>
          <p className="mt-1 text-sm text-gray-500">
            All proposals have been processed.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header with counts */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h2 className="text-lg font-medium text-gray-200">
            Pending Proposals
            <span className="ml-2 text-sm text-gray-500">({proposals.length})</span>
          </h2>
          {urgentCount > 0 && (
            <span className="flex items-center gap-1 px-2 py-1 bg-red-900/40 text-red-300 text-xs font-medium rounded animate-pulse">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                />
              </svg>
              {urgentCount} expiring soon
            </span>
          )}
        </div>
        <div className="flex items-center gap-4">
          <div className="flex items-center gap-2 text-xs">
            {groupedProposals.critical.length > 0 && (
              <span className="px-2 py-1 bg-red-900/40 text-red-300 rounded">
                {groupedProposals.critical.length} Critical
              </span>
            )}
            {groupedProposals.high.length > 0 && (
              <span className="px-2 py-1 bg-orange-900/40 text-orange-300 rounded">
                {groupedProposals.high.length} High
              </span>
            )}
            {groupedProposals.medium.length > 0 && (
              <span className="px-2 py-1 bg-sky-900/40 text-sky-300 rounded">
                {groupedProposals.medium.length} Medium
              </span>
            )}
          </div>
          {/* View mode toggle */}
          <div className="flex items-center gap-1 bg-gray-800 rounded-lg p-1">
            <button
              onClick={() => setViewMode('card')}
              className={clsx(
                'px-3 py-1.5 rounded transition-colors flex items-center gap-1.5',
                viewMode === 'card'
                  ? 'bg-gray-700 text-gray-200'
                  : 'text-gray-400 hover:text-gray-300'
              )}
              title="Card view"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z"
                />
              </svg>
              <span className="text-xs font-medium">Cards</span>
            </button>
            <button
              onClick={() => setViewMode('list')}
              className={clsx(
                'px-3 py-1.5 rounded transition-colors flex items-center gap-1.5',
                viewMode === 'list'
                  ? 'bg-gray-700 text-gray-200'
                  : 'text-gray-400 hover:text-gray-300'
              )}
              title="List view"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M3 10h18M3 14h18m-9-4v8m-7 0h14a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"
                />
              </svg>
              <span className="text-xs font-medium">List</span>
            </button>
          </div>
        </div>
      </div>

      {/* Conditional rendering based on view mode */}
      {viewMode === 'card' ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {proposals.map((proposal) => (
            <ProposalCard
              key={proposal.proposal_id}
              proposal={proposal}
              onApprove={() => onApprove(proposal.proposal_id)}
              onDeny={() => onDeny(proposal.proposal_id)}
            />
          ))}
        </div>
      ) : (
        <ProposalListView
          proposals={proposals}
          onApprove={onApprove}
          onDeny={onDeny}
        />
      )}
    </div>
  );
}

export default ProposalQueue;
