import { useState, useEffect, useCallback } from 'react';
import clsx from 'clsx';
import { useInterventionRules } from '../hooks/useInterventionRules';
import { ConfirmationModal } from './ConfirmationModal';
import type { InterventionRule, InterventionRuleCreate, InterventionRuleUpdate } from '../types';

// Available options for multi-selects
const ACTION_TYPES = ['engage', 'track', 'identify', 'ignore', 'intercept', 'monitor'];
const THREAT_LEVELS = ['critical', 'high', 'medium', 'low', 'unknown'];
const CLASSIFICATIONS = ['friendly', 'hostile', 'neutral', 'unknown'];
const TRACK_TYPES = ['aircraft', 'vessel', 'ground', 'missile', 'unknown'];

// Color mapping for action type badges
const actionTypeColors: Record<string, string> = {
  engage: 'bg-red-900/40 text-red-300 border-red-500/30',
  intercept: 'bg-orange-900/40 text-orange-300 border-orange-500/30',
  track: 'bg-blue-900/40 text-blue-300 border-blue-500/30',
  identify: 'bg-cyan-900/40 text-cyan-300 border-cyan-500/30',
  monitor: 'bg-green-900/40 text-green-300 border-green-500/30',
  ignore: 'bg-gray-800 text-gray-400 border-gray-600',
};

// Color mapping for threat level badges
const threatLevelColors: Record<string, string> = {
  critical: 'bg-red-900/40 text-red-300',
  high: 'bg-orange-900/40 text-orange-300',
  medium: 'bg-yellow-900/40 text-yellow-300',
  low: 'bg-green-900/40 text-green-300',
  unknown: 'bg-gray-700 text-gray-300',
};

// Badge component for displaying arrays of values
function BadgeList({ items, colorMap, maxDisplay = 3 }: { items: string[]; colorMap: Record<string, string>; maxDisplay?: number }) {
  if (!items || items.length === 0) {
    return <span className="text-gray-500 text-xs">Any</span>;
  }

  const displayItems = items.slice(0, maxDisplay);
  const remaining = items.length - maxDisplay;

  return (
    <div className="flex flex-wrap gap-1">
      {displayItems.map((item) => (
        <span
          key={item}
          className={clsx(
            'px-1.5 py-0.5 text-xs rounded border',
            colorMap[item] || 'bg-gray-700 text-gray-300 border-gray-600'
          )}
        >
          {item}
        </span>
      ))}
      {remaining > 0 && (
        <span className="px-1.5 py-0.5 text-xs bg-gray-700 text-gray-400 rounded">
          +{remaining}
        </span>
      )}
    </div>
  );
}

// Multi-select component for checkboxes
interface MultiSelectProps {
  label: string;
  options: string[];
  selected: string[];
  onChange: (selected: string[]) => void;
  colorMap?: Record<string, string>;
}

function MultiSelect({ label, options, selected, onChange, colorMap }: MultiSelectProps) {
  const toggleOption = (option: string) => {
    if (selected.includes(option)) {
      onChange(selected.filter((s) => s !== option));
    } else {
      onChange([...selected, option]);
    }
  };

  return (
    <div>
      <label className="block text-xs font-medium text-gray-400 uppercase mb-2">{label}</label>
      <div className="flex flex-wrap gap-2">
        {options.map((option) => {
          const isSelected = selected.includes(option);
          const baseColor = colorMap?.[option] || '';
          return (
            <button
              key={option}
              type="button"
              onClick={() => toggleOption(option)}
              className={clsx(
                'px-2.5 py-1.5 text-xs rounded border transition-all',
                isSelected
                  ? colorMap
                    ? `${baseColor} border-current`
                    : 'bg-green-900/40 text-green-300 border-green-500'
                  : 'bg-gray-800 text-gray-400 border-gray-700 hover:border-gray-600'
              )}
            >
              {option}
            </button>
          );
        })}
      </div>
      {selected.length === 0 && (
        <p className="mt-1 text-xs text-gray-500">No selection means rule applies to all</p>
      )}
    </div>
  );
}

// Toggle switch component
interface ToggleProps {
  label: string;
  description?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}

function Toggle({ label, description, checked, onChange, disabled }: ToggleProps) {
  return (
    <label className={clsx('flex items-start gap-3 cursor-pointer', disabled && 'opacity-50 cursor-not-allowed')}>
      <div className="relative flex-shrink-0 mt-0.5">
        <input
          type="checkbox"
          className="sr-only"
          checked={checked}
          onChange={(e) => onChange(e.target.checked)}
          disabled={disabled}
        />
        <div
          className={clsx(
            'w-10 h-6 rounded-full transition-colors',
            checked ? 'bg-green-600' : 'bg-gray-700'
          )}
        />
        <div
          className={clsx(
            'absolute top-1 w-4 h-4 rounded-full bg-white transition-transform',
            checked ? 'translate-x-5' : 'translate-x-1'
          )}
        />
      </div>
      <div>
        <span className="text-sm text-gray-200">{label}</span>
        {description && <p className="text-xs text-gray-500">{description}</p>}
      </div>
    </label>
  );
}

// Rule form modal
interface RuleModalProps {
  isOpen: boolean;
  editingRule: InterventionRule | null;
  isSubmitting: boolean;
  onSubmit: (rule: InterventionRuleCreate | InterventionRuleUpdate) => void;
  onClose: () => void;
}

function RuleModal({ isOpen, editingRule, isSubmitting, onSubmit, onClose }: RuleModalProps) {
  const [formData, setFormData] = useState<InterventionRuleCreate>({
    name: '',
    description: '',
    action_types: [],
    threat_levels: [],
    classifications: [],
    track_types: [],
    min_priority: undefined,
    max_priority: undefined,
    requires_approval: true,
    auto_approve: false,
    enabled: true,
    evaluation_order: 100,
  });

  // Reset form when modal opens or editingRule changes
  useEffect(() => {
    if (isOpen) {
      if (editingRule) {
        setFormData({
          name: editingRule.name,
          description: editingRule.description || '',
          action_types: editingRule.action_types || [],
          threat_levels: editingRule.threat_levels || [],
          classifications: editingRule.classifications || [],
          track_types: editingRule.track_types || [],
          min_priority: editingRule.min_priority ?? undefined,
          max_priority: editingRule.max_priority ?? undefined,
          requires_approval: editingRule.requires_approval,
          auto_approve: editingRule.auto_approve,
          enabled: editingRule.enabled,
          evaluation_order: editingRule.evaluation_order,
        });
      } else {
        setFormData({
          name: '',
          description: '',
          action_types: [],
          threat_levels: [],
          classifications: [],
          track_types: [],
          min_priority: undefined,
          max_priority: undefined,
          requires_approval: true,
          auto_approve: false,
          enabled: true,
          evaluation_order: 100,
        });
      }
    }
  }, [isOpen, editingRule]);

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

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit(formData);
  };

  if (!isOpen) return null;

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
          role="dialog"
          aria-modal="true"
          aria-labelledby="rule-modal-title"
          className="relative w-full max-w-2xl bg-gray-900 rounded-lg shadow-2xl border border-gray-700"
        >
          {/* Header */}
          <div className="px-6 py-4 border-b border-gray-700 bg-gray-800/50">
            <div className="flex items-center justify-between">
              <h2 id="rule-modal-title" className="text-lg font-semibold text-gray-100">
                {editingRule ? 'Edit Intervention Rule' : 'Create Intervention Rule'}
              </h2>
              <button
                onClick={onClose}
                disabled={isSubmitting}
                aria-label="Close modal"
                className="text-gray-500 hover:text-gray-300 disabled:opacity-50"
              >
                <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          {/* Form */}
          <form onSubmit={handleSubmit}>
            <div className="px-6 py-4 space-y-5 max-h-[calc(100vh-200px)] overflow-y-auto">
              {/* Name */}
              <div>
                <label className="block text-xs font-medium text-gray-400 uppercase mb-1">
                  Name <span className="text-red-400">*</span>
                </label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  disabled={isSubmitting}
                  placeholder="e.g., Auto-approve low threat monitoring"
                  className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500 disabled:opacity-50"
                />
              </div>

              {/* Description */}
              <div>
                <label className="block text-xs font-medium text-gray-400 uppercase mb-1">
                  Description
                </label>
                <textarea
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  disabled={isSubmitting}
                  placeholder="Optional description of what this rule does..."
                  rows={2}
                  className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500 disabled:opacity-50 resize-none"
                />
              </div>

              {/* Action Types */}
              <MultiSelect
                label="Action Types"
                options={ACTION_TYPES}
                selected={formData.action_types}
                onChange={(selected) => setFormData({ ...formData, action_types: selected })}
                colorMap={actionTypeColors}
              />

              {/* Threat Levels */}
              <MultiSelect
                label="Threat Levels"
                options={THREAT_LEVELS}
                selected={formData.threat_levels || []}
                onChange={(selected) => setFormData({ ...formData, threat_levels: selected })}
                colorMap={threatLevelColors}
              />

              {/* Classifications */}
              <MultiSelect
                label="Classifications"
                options={CLASSIFICATIONS}
                selected={formData.classifications || []}
                onChange={(selected) => setFormData({ ...formData, classifications: selected })}
              />

              {/* Track Types */}
              <MultiSelect
                label="Track Types"
                options={TRACK_TYPES}
                selected={formData.track_types || []}
                onChange={(selected) => setFormData({ ...formData, track_types: selected })}
              />

              {/* Priority Range */}
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-gray-400 uppercase mb-1">
                    Min Priority (1-10)
                  </label>
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={formData.min_priority ?? ''}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        min_priority: e.target.value ? parseInt(e.target.value, 10) : undefined,
                      })
                    }
                    disabled={isSubmitting}
                    placeholder="Any"
                    className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500 disabled:opacity-50"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-gray-400 uppercase mb-1">
                    Max Priority (1-10)
                  </label>
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={formData.max_priority ?? ''}
                    onChange={(e) =>
                      setFormData({
                        ...formData,
                        max_priority: e.target.value ? parseInt(e.target.value, 10) : undefined,
                      })
                    }
                    disabled={isSubmitting}
                    placeholder="Any"
                    className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500 disabled:opacity-50"
                  />
                </div>
              </div>

              {/* Toggles */}
              <div className="space-y-4 pt-2">
                <Toggle
                  label="Requires Approval"
                  description="If enabled, proposals matching this rule require human approval"
                  checked={formData.requires_approval}
                  onChange={(checked) => setFormData({ ...formData, requires_approval: checked })}
                  disabled={isSubmitting}
                />

                <Toggle
                  label="Auto Approve"
                  description="If enabled and requires_approval is false, automatically approve matching proposals"
                  checked={formData.auto_approve || false}
                  onChange={(checked) => setFormData({ ...formData, auto_approve: checked })}
                  disabled={isSubmitting || formData.requires_approval}
                />

                <Toggle
                  label="Enabled"
                  description="Rule is active and will be evaluated"
                  checked={formData.enabled ?? true}
                  onChange={(checked) => setFormData({ ...formData, enabled: checked })}
                  disabled={isSubmitting}
                />
              </div>

              {/* Evaluation Order */}
              <div>
                <label className="block text-xs font-medium text-gray-400 uppercase mb-1">
                  Evaluation Order
                </label>
                <input
                  type="number"
                  min={0}
                  value={formData.evaluation_order ?? 100}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      evaluation_order: parseInt(e.target.value, 10) || 100,
                    })
                  }
                  disabled={isSubmitting}
                  className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500 disabled:opacity-50"
                />
                <p className="mt-1 text-xs text-gray-500">
                  Lower numbers are evaluated first. Rules with the same order are evaluated by creation time.
                </p>
              </div>
            </div>

            {/* Footer */}
            <div className="px-6 py-4 border-t border-gray-700 flex items-center justify-end gap-3">
              <button
                type="button"
                onClick={onClose}
                disabled={isSubmitting}
                className="px-4 py-2 bg-gray-700 hover:bg-gray-600 text-gray-300 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={isSubmitting || !formData.name}
                className="px-6 py-2 bg-green-600 hover:bg-green-500 text-white font-medium rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
              >
                {isSubmitting ? (
                  <>
                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white" />
                    Saving...
                  </>
                ) : editingRule ? (
                  'Save Changes'
                ) : (
                  'Create Rule'
                )}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}

// Main panel component
export function InterventionRulesPanel() {
  const {
    rules,
    ruleCount,
    isLoading,
    error,
    filter,
    sortConfig,
    modalOpen,
    editingRule,
    deleteConfirmOpen,
    deleteTargetId,
    isCreating,
    isUpdating,
    isDeleting,
    setFilter,
    toggleSort,
    openModal,
    closeModal,
    openDeleteConfirm,
    closeDeleteConfirm,
    createRule,
    updateRule,
    deleteRule,
    toggleRuleEnabled,
    getRuleById,
  } = useInterventionRules();

  const [searchInput, setSearchInput] = useState('');

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => {
      setFilter({ searchQuery: searchInput });
    }, 300);
    return () => clearTimeout(timer);
  }, [searchInput, setFilter]);

  const handleSubmit = useCallback(
    async (data: InterventionRuleCreate | InterventionRuleUpdate) => {
      if (editingRule) {
        await updateRule(editingRule.rule_id, data);
      } else {
        await createRule(data as InterventionRuleCreate);
      }
    },
    [editingRule, createRule, updateRule]
  );

  const handleDelete = useCallback(async () => {
    if (deleteTargetId) {
      await deleteRule(deleteTargetId);
    }
  }, [deleteTargetId, deleteRule]);

  const deleteTargetRule = deleteTargetId ? getRuleById(deleteTargetId) : null;

  // Render sort indicator
  const renderSortIndicator = (key: string) => {
    if (sortConfig.key !== key) return null;
    return (
      <svg
        className={clsx('w-4 h-4 ml-1', sortConfig.direction === 'desc' && 'rotate-180')}
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
      </svg>
    );
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
          <span className="text-gray-400 text-sm">Loading intervention rules...</span>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-red-900/50">
        <div className="text-center">
          <svg className="mx-auto h-12 w-12 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1}
              d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
            />
          </svg>
          <h3 className="mt-2 text-sm font-medium text-red-400">Failed to load rules</h3>
          <p className="mt-1 text-sm text-gray-500">{String(error)}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <h2 className="text-lg font-medium text-gray-200">
            Intervention Rules
            <span className="ml-2 text-sm text-gray-500">({ruleCount})</span>
          </h2>
        </div>
        <button
          onClick={() => openModal()}
          className="flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-500 text-white text-sm font-medium rounded transition-colors"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          Create Rule
        </button>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-4">
        {/* Search */}
        <div className="relative flex-1 max-w-md">
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-500"
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
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="Search rules..."
            className="w-full pl-10 pr-4 py-2 bg-gray-800 border border-gray-700 rounded text-gray-200 placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-green-500/50 focus:border-green-500 text-sm"
          />
        </div>

        {/* Enabled filter */}
        <div className="flex items-center gap-1 bg-gray-800 rounded-lg p-1">
          <button
            onClick={() => setFilter({ enabled: null })}
            className={clsx(
              'px-3 py-1.5 rounded text-xs font-medium transition-colors',
              filter.enabled === null ? 'bg-gray-700 text-gray-200' : 'text-gray-400 hover:text-gray-300'
            )}
          >
            All
          </button>
          <button
            onClick={() => setFilter({ enabled: true })}
            className={clsx(
              'px-3 py-1.5 rounded text-xs font-medium transition-colors',
              filter.enabled === true ? 'bg-green-900/40 text-green-300' : 'text-gray-400 hover:text-gray-300'
            )}
          >
            Enabled
          </button>
          <button
            onClick={() => setFilter({ enabled: false })}
            className={clsx(
              'px-3 py-1.5 rounded text-xs font-medium transition-colors',
              filter.enabled === false ? 'bg-red-900/40 text-red-300' : 'text-gray-400 hover:text-gray-300'
            )}
          >
            Disabled
          </button>
        </div>
      </div>

      {/* Table */}
      {rules.length === 0 ? (
        <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
          <div className="text-center">
            <svg className="mx-auto h-12 w-12 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={1}
                d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
              />
            </svg>
            <h3 className="mt-2 text-sm font-medium text-gray-400">No intervention rules</h3>
            <p className="mt-1 text-sm text-gray-500">
              {filter.enabled !== null || filter.searchQuery
                ? 'No rules match your filters.'
                : 'Get started by creating a new rule.'}
            </p>
          </div>
        </div>
      ) : (
        <div className="bg-gray-900 border border-gray-700 rounded-lg overflow-hidden">
          <table className="w-full">
            <thead className="bg-gray-800/50 border-b border-gray-700">
              <tr>
                <th
                  className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider cursor-pointer hover:text-gray-200"
                  onClick={() => toggleSort('name')}
                >
                  <div className="flex items-center">
                    Name
                    {renderSortIndicator('name')}
                  </div>
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Action Types
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Threat Levels
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Classifications
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Priority
                </th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Approval
                </th>
                <th
                  className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider cursor-pointer hover:text-gray-200"
                  onClick={() => toggleSort('enabled')}
                >
                  <div className="flex items-center">
                    Status
                    {renderSortIndicator('enabled')}
                  </div>
                </th>
                <th className="px-4 py-3 text-right text-xs font-medium text-gray-400 uppercase tracking-wider">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-700">
              {rules.map((rule) => (
                <tr key={rule.rule_id} className="hover:bg-gray-800/50 transition-colors">
                  <td className="px-4 py-3">
                    <div>
                      <span className="text-sm font-medium text-gray-200">{rule.name}</span>
                      {rule.description && (
                        <p className="text-xs text-gray-500 mt-0.5 truncate max-w-xs">{rule.description}</p>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <BadgeList items={rule.action_types} colorMap={actionTypeColors} />
                  </td>
                  <td className="px-4 py-3">
                    <BadgeList items={rule.threat_levels} colorMap={threatLevelColors} />
                  </td>
                  <td className="px-4 py-3">
                    <BadgeList items={rule.classifications} colorMap={{}} maxDisplay={2} />
                  </td>
                  <td className="px-4 py-3">
                    <span className="text-xs text-gray-400">
                      {rule.min_priority || rule.max_priority
                        ? `${rule.min_priority ?? 1}-${rule.max_priority ?? 10}`
                        : 'Any'}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={clsx(
                        'px-2 py-1 text-xs rounded',
                        rule.requires_approval
                          ? 'bg-yellow-900/40 text-yellow-300'
                          : rule.auto_approve
                          ? 'bg-green-900/40 text-green-300'
                          : 'bg-gray-700 text-gray-400'
                      )}
                    >
                      {rule.requires_approval ? 'Required' : rule.auto_approve ? 'Auto' : 'Skip'}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => toggleRuleEnabled(rule)}
                      className={clsx(
                        'px-2 py-1 text-xs rounded transition-colors',
                        rule.enabled
                          ? 'bg-green-900/40 text-green-300 hover:bg-green-900/60'
                          : 'bg-red-900/40 text-red-300 hover:bg-red-900/60'
                      )}
                    >
                      {rule.enabled ? 'Enabled' : 'Disabled'}
                    </button>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        onClick={() => openModal(rule)}
                        className="p-1.5 text-gray-400 hover:text-gray-200 hover:bg-gray-700 rounded transition-colors"
                        title="Edit rule"
                      >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                          />
                        </svg>
                      </button>
                      <button
                        onClick={() => openDeleteConfirm(rule.rule_id)}
                        className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-red-900/30 rounded transition-colors"
                        title="Delete rule"
                      >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={2}
                            d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                          />
                        </svg>
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Create/Edit Modal */}
      <RuleModal
        isOpen={modalOpen}
        editingRule={editingRule}
        isSubmitting={isCreating || isUpdating}
        onSubmit={handleSubmit}
        onClose={closeModal}
      />

      {/* Delete Confirmation Modal */}
      <ConfirmationModal
        isOpen={deleteConfirmOpen}
        onClose={closeDeleteConfirm}
        onConfirm={handleDelete}
        title="Delete Intervention Rule"
        message={`Are you sure you want to delete "${deleteTargetRule?.name || 'this rule'}"? This action cannot be undone.`}
        confirmText="Delete Rule"
        cancelText="Cancel"
        variant="danger"
        isLoading={isDeleting}
      />
    </div>
  );
}

export default InterventionRulesPanel;
