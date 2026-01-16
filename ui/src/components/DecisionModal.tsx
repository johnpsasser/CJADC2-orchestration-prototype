import { useState, useCallback, useEffect } from 'react';
import { formatDistanceToNow, differenceInSeconds } from 'date-fns';
import clsx from 'clsx';
import type { ActionProposal, ThreatLevel } from '../types';

interface DecisionModalProps {
  proposal: ActionProposal | null;
  isOpen: boolean;
  isSubmitting: boolean;
  initialDecision?: 'approve' | 'deny' | null;
  onApprove: (reason: string, conditions?: string[]) => void;
  onDeny: (reason: string) => void;
  onClose: () => void;
}

// Threat level colors
const threatLevelColors: Record<ThreatLevel, { bg: string; text: string; border: string }> = {
  critical: { bg: 'bg-red-900/30', text: 'text-red-400', border: 'border-red-500' },
  high: { bg: 'bg-orange-900/30', text: 'text-orange-400', border: 'border-orange-500' },
  medium: { bg: 'bg-yellow-900/30', text: 'text-yellow-400', border: 'border-yellow-500' },
  low: { bg: 'bg-green-900/30', text: 'text-green-400', border: 'border-green-500' },
  unknown: { bg: 'bg-gray-900/30', text: 'text-gray-400', border: 'border-gray-500' },
};

// Countdown timer
function ExpirationTimer({ expiresAt }: { expiresAt: string }) {
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
    return (
      <div className="flex items-center gap-2 text-red-500">
        <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
          />
        </svg>
        <span className="font-medium">EXPIRED</span>
      </div>
    );
  }

  return (
    <div
      className={clsx(
        'flex items-center gap-2 px-3 py-1 rounded-full font-mono text-sm',
        isCritical
          ? 'bg-red-900/40 text-red-300 animate-pulse'
          : isUrgent
          ? 'bg-orange-900/40 text-orange-300'
          : 'bg-gray-800 text-gray-300'
      )}
    >
      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path
          strokeLinecap="round"
          strokeLinejoin="round"
          strokeWidth={2}
          d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
      <span>
        {minutes}:{seconds.toString().padStart(2, '0')}
      </span>
    </div>
  );
}

// Predefined reasons for decisions
const APPROVE_REASONS = [
  'Threat assessment validated - action authorized',
  'Policy requirements met - proceeding as recommended',
  'Situation requires immediate response',
  'Operator judgment - conditions acceptable',
  'Command authority override',
] as const;

const DENY_REASONS = [
  'Insufficient threat evidence',
  'Policy violation - action not permitted',
  'Collateral risk too high',
  'Alternative action preferred',
  'Awaiting additional intelligence',
  'Command authority override - denied',
] as const;

export function DecisionModal({
  proposal,
  isOpen,
  isSubmitting,
  initialDecision,
  onApprove,
  onDeny,
  onClose,
}: DecisionModalProps) {
  const [selectedReason, setSelectedReason] = useState<string>(APPROVE_REASONS[0]);
  const [additionalContext, setAdditionalContext] = useState('');
  const [conditions, setConditions] = useState('');
  const [decision, setDecision] = useState<'approve' | 'deny' | null>(null);
  const [error, setError] = useState('');

  // Reset form when modal opens with new proposal
  useEffect(() => {
    if (isOpen && proposal) {
      // Use the initial decision if provided (from Approve/Deny button click)
      const startingDecision = initialDecision ?? null;
      setDecision(startingDecision);
      setSelectedReason(startingDecision === 'deny' ? DENY_REASONS[0] : APPROVE_REASONS[0]);
      setAdditionalContext('');
      setConditions('');
      setError('');
    }
  }, [isOpen, proposal?.proposal_id, initialDecision]);

  // Update default reason when decision type changes
  useEffect(() => {
    if (decision === 'approve') {
      setSelectedReason(APPROVE_REASONS[0]);
    } else if (decision === 'deny') {
      setSelectedReason(DENY_REASONS[0]);
    }
  }, [decision]);

  // Handle escape key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && isOpen && !isSubmitting) {
        onClose();
      }
    };
    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [isOpen, isSubmitting, onClose]);

  const handleSubmit = useCallback(() => {
    if (!decision) {
      setError('Please select approve or deny');
      return;
    }

    setError('');

    // Combine selected reason with additional context
    const fullReason = additionalContext.trim()
      ? `${selectedReason}. Additional context: ${additionalContext.trim()}`
      : selectedReason;

    if (decision === 'approve') {
      const conditionsList = conditions
        .split('\n')
        .map((c) => c.trim())
        .filter((c) => c.length > 0);
      onApprove(fullReason, conditionsList.length > 0 ? conditionsList : undefined);
    } else {
      onDeny(fullReason);
    }
  }, [selectedReason, additionalContext, conditions, decision, onApprove, onDeny]);

  if (!isOpen || !proposal) return null;

  const colors = threatLevelColors[proposal.threat_level];

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/70 backdrop-blur-sm transition-opacity"
        onClick={!isSubmitting ? onClose : undefined}
      />

      {/* Modal */}
      <div className="flex min-h-full items-center justify-center p-4">
        <div
          className={clsx(
            'relative w-full max-w-2xl bg-gray-900 rounded-lg shadow-2xl border',
            colors.border
          )}
        >
          {/* Header */}
          <div className={clsx('px-6 py-4 border-b border-gray-700', colors.bg)}>
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-lg font-semibold text-gray-100">Decision Required</h2>
                <p className="text-sm text-gray-400">
                  Proposal ID: <span className="font-mono">{proposal.proposal_id}</span>
                </p>
              </div>
              <div className="flex items-center gap-3">
                <ExpirationTimer expiresAt={proposal.expires_at} />
                <button
                  onClick={onClose}
                  disabled={isSubmitting}
                  className="text-gray-500 hover:text-gray-300 disabled:opacity-50"
                >
                  <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M6 18L18 6M6 6l12 12"
                    />
                  </svg>
                </button>
              </div>
            </div>
          </div>

          {/* Content */}
          <div className="px-6 py-4 space-y-4">
            {/* Proposal Summary */}
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                  Action Type
                </label>
                <span className="text-lg font-semibold text-gray-200 uppercase">
                  {proposal.action_type}
                </span>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                  Priority
                </label>
                <span
                  className={clsx(
                    'inline-block px-2 py-1 text-sm font-medium rounded',
                    proposal.priority >= 9
                      ? 'bg-red-900/40 text-red-300'
                      : proposal.priority >= 7
                      ? 'bg-orange-900/40 text-orange-300'
                      : proposal.priority >= 4
                      ? 'bg-yellow-900/40 text-yellow-300'
                      : 'bg-gray-700 text-gray-300'
                  )}
                >
                  Level {proposal.priority}
                </span>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                  Track ID
                </label>
                <span className="font-mono text-gray-300">{proposal.track_id}</span>
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                  Threat Level
                </label>
                <span
                  className={clsx('inline-block px-2 py-1 text-sm font-medium rounded', colors.bg, colors.text)}
                >
                  {proposal.threat_level.toUpperCase()}
                </span>
              </div>
            </div>

            {/* Rationale */}
            <div>
              <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                Rationale
              </label>
              <p className="text-sm text-gray-300 bg-gray-800 rounded p-3">
                {proposal.rationale}
              </p>
            </div>

            {/* Policy Decision Info */}
            {proposal.policy_decision && (
              <div className="bg-gray-800 rounded p-3 space-y-2">
                <div className="flex items-center gap-2">
                  <span
                    className={clsx(
                      'w-2 h-2 rounded-full',
                      proposal.policy_decision.allowed ? 'bg-green-500' : 'bg-red-500'
                    )}
                  />
                  <span className="text-xs font-medium text-gray-400 uppercase">
                    Policy Decision: {proposal.policy_decision.allowed ? 'Allowed' : 'Denied'}
                  </span>
                </div>
                {proposal.policy_decision.reasons && proposal.policy_decision.reasons.length > 0 && (
                  <div>
                    <span className="text-xs text-gray-500">Reasons:</span>
                    <ul className="mt-1 list-disc list-inside text-xs text-gray-400">
                      {proposal.policy_decision.reasons.map((r, i) => (
                        <li key={i}>{r}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {proposal.policy_decision.violations && proposal.policy_decision.violations.length > 0 && (
                  <div>
                    <span className="text-xs text-red-400">Violations:</span>
                    <ul className="mt-1 list-disc list-inside text-xs text-red-400">
                      {proposal.policy_decision.violations.map((v, i) => (
                        <li key={i}>{v}</li>
                      ))}
                    </ul>
                  </div>
                )}
                {proposal.policy_decision.warnings && proposal.policy_decision.warnings.length > 0 && (
                  <div>
                    <span className="text-xs text-yellow-400">Warnings:</span>
                    <ul className="mt-1 list-disc list-inside text-xs text-yellow-400">
                      {proposal.policy_decision.warnings.map((w, i) => (
                        <li key={i}>{w}</li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}

            {/* Decision Selection */}
            <div className="flex gap-4">
              <button
                onClick={() => setDecision('approve')}
                disabled={isSubmitting}
                className={clsx(
                  'flex-1 py-3 px-4 rounded-lg border-2 transition-all font-medium',
                  decision === 'approve'
                    ? 'border-green-500 bg-green-900/30 text-green-300'
                    : 'border-gray-700 bg-gray-800 text-gray-400 hover:border-green-600 hover:text-green-400',
                  isSubmitting && 'opacity-50 cursor-not-allowed'
                )}
              >
                <div className="flex items-center justify-center gap-2">
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M5 13l4 4L19 7"
                    />
                  </svg>
                  APPROVE
                </div>
              </button>
              <button
                onClick={() => setDecision('deny')}
                disabled={isSubmitting}
                className={clsx(
                  'flex-1 py-3 px-4 rounded-lg border-2 transition-all font-medium',
                  decision === 'deny'
                    ? 'border-red-500 bg-red-900/30 text-red-300'
                    : 'border-gray-700 bg-gray-800 text-gray-400 hover:border-red-600 hover:text-red-400',
                  isSubmitting && 'opacity-50 cursor-not-allowed'
                )}
              >
                <div className="flex items-center justify-center gap-2">
                  <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M6 18L18 6M6 6l12 12"
                    />
                  </svg>
                  DENY
                </div>
              </button>
            </div>

            {/* Reason Selection */}
            {decision && (
              <div className="space-y-3">
                <div>
                  <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                    Reason <span className="text-red-400">*</span>
                  </label>
                  <select
                    value={selectedReason}
                    onChange={(e) => setSelectedReason(e.target.value)}
                    disabled={isSubmitting}
                    className={clsx(
                      'w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200',
                      'focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500',
                      isSubmitting && 'opacity-50 cursor-not-allowed'
                    )}
                  >
                    {(decision === 'approve' ? APPROVE_REASONS : DENY_REASONS).map((reason) => (
                      <option key={reason} value={reason}>
                        {reason}
                      </option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                    Additional Context (Optional)
                  </label>
                  <textarea
                    value={additionalContext}
                    onChange={(e) => setAdditionalContext(e.target.value)}
                    disabled={isSubmitting}
                    placeholder="Add any additional context or notes..."
                    rows={2}
                    className={clsx(
                      'w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 resize-none',
                      'focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500',
                      isSubmitting && 'opacity-50 cursor-not-allowed'
                    )}
                  />
                </div>
              </div>
            )}

            {/* Conditions (only for approve) */}
            {decision === 'approve' && (
              <div>
                <label className="block text-xs font-medium text-gray-500 uppercase mb-1">
                  Conditions (Optional, one per line)
                </label>
                <textarea
                  value={conditions}
                  onChange={(e) => setConditions(e.target.value)}
                  disabled={isSubmitting}
                  placeholder="Enter any conditions for approval..."
                  rows={2}
                  className={clsx(
                    'w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 resize-none',
                    'focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500',
                    isSubmitting && 'opacity-50 cursor-not-allowed'
                  )}
                />
              </div>
            )}

            {/* Error */}
            {error && (
              <div className="flex items-center gap-2 text-red-400 text-sm">
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                {error}
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="px-6 py-4 border-t border-gray-700 flex items-center justify-end gap-3">
            <button
              onClick={onClose}
              disabled={isSubmitting}
              className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-gray-300 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Cancel
            </button>
            <button
              onClick={handleSubmit}
              disabled={isSubmitting || !decision}
              className={clsx(
                'px-6 py-2 font-medium rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed',
                decision === 'approve'
                  ? 'bg-green-600 hover:bg-green-500 text-white'
                  : decision === 'deny'
                  ? 'bg-red-600 hover:bg-red-500 text-white'
                  : 'bg-gray-600 text-gray-400'
              )}
            >
              {isSubmitting ? (
                <div className="flex items-center gap-2">
                  <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                  Submitting...
                </div>
              ) : (
                'Submit Decision'
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default DecisionModal;
