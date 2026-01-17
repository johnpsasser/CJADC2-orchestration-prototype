import { useCallback, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { create } from 'zustand';
import { api } from '../api/client';
import type { InterventionRule, InterventionRuleCreate, InterventionRuleUpdate, SortConfig } from '../types';

// Zustand store for intervention rules UI state
interface InterventionRulesStore {
  selectedRuleId: string | null;
  filter: {
    enabled: boolean | null;
    searchQuery: string;
  };
  sortConfig: SortConfig;
  modalOpen: boolean;
  editingRule: InterventionRule | null;
  deleteConfirmOpen: boolean;
  deleteTargetId: string | null;
  selectRule: (ruleId: string | null) => void;
  setFilter: (filter: Partial<InterventionRulesStore['filter']>) => void;
  clearFilter: () => void;
  setSortConfig: (config: SortConfig) => void;
  openModal: (rule?: InterventionRule) => void;
  closeModal: () => void;
  openDeleteConfirm: (ruleId: string) => void;
  closeDeleteConfirm: () => void;
}

export const useInterventionRulesStore = create<InterventionRulesStore>((set) => ({
  selectedRuleId: null,
  filter: {
    enabled: null,
    searchQuery: '',
  },
  sortConfig: { key: 'evaluation_order', direction: 'asc' },
  modalOpen: false,
  editingRule: null,
  deleteConfirmOpen: false,
  deleteTargetId: null,
  selectRule: (ruleId) => set({ selectedRuleId: ruleId }),
  setFilter: (filter) =>
    set((state) => ({
      filter: { ...state.filter, ...filter },
    })),
  clearFilter: () =>
    set({
      filter: {
        enabled: null,
        searchQuery: '',
      },
    }),
  setSortConfig: (config) => set({ sortConfig: config }),
  openModal: (rule) => set({ modalOpen: true, editingRule: rule ?? null }),
  closeModal: () => set({ modalOpen: false, editingRule: null }),
  openDeleteConfirm: (ruleId) => set({ deleteConfirmOpen: true, deleteTargetId: ruleId }),
  closeDeleteConfirm: () => set({ deleteConfirmOpen: false, deleteTargetId: null }),
}));

// Query key for intervention rules
const RULES_QUERY_KEY = ['interventionRules'];

// Hook for fetching and managing intervention rules
export function useInterventionRules() {
  const queryClient = useQueryClient();
  const {
    selectedRuleId,
    filter,
    sortConfig,
    modalOpen,
    editingRule,
    deleteConfirmOpen,
    deleteTargetId,
    selectRule,
    setFilter,
    clearFilter,
    setSortConfig,
    openModal,
    closeModal,
    openDeleteConfirm,
    closeDeleteConfirm,
  } = useInterventionRulesStore();

  // Fetch intervention rules from API
  const {
    data: rules = [],
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: RULES_QUERY_KEY,
    queryFn: async () => {
      const response = await api.interventionRules.getAll();
      return response.data;
    },
    refetchInterval: 30000, // Refetch every 30 seconds (rules don't change as frequently)
    staleTime: 15000,
  });

  // Create rule mutation
  const createMutation = useMutation({
    mutationFn: async (rule: InterventionRuleCreate) => {
      const response = await api.interventionRules.create(rule);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RULES_QUERY_KEY });
      closeModal();
    },
  });

  // Update rule mutation
  const updateMutation = useMutation({
    mutationFn: async ({ ruleId, rule }: { ruleId: string; rule: InterventionRuleUpdate }) => {
      const response = await api.interventionRules.update(ruleId, rule);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: RULES_QUERY_KEY });
      closeModal();
    },
  });

  // Delete rule mutation
  const deleteMutation = useMutation({
    mutationFn: async (ruleId: string) => {
      await api.interventionRules.delete(ruleId);
      return ruleId;
    },
    onSuccess: (deletedId) => {
      queryClient.invalidateQueries({ queryKey: RULES_QUERY_KEY });
      closeDeleteConfirm();
      if (selectedRuleId === deletedId) {
        selectRule(null);
      }
    },
  });

  // Get filtered and sorted rules
  const filteredRules = useMemo(() => {
    let result = [...rules];

    // Apply enabled filter
    if (filter.enabled !== null) {
      result = result.filter((rule) => rule.enabled === filter.enabled);
    }

    // Apply search filter
    if (filter.searchQuery) {
      const query = filter.searchQuery.toLowerCase();
      result = result.filter(
        (rule) =>
          rule.name.toLowerCase().includes(query) ||
          (rule.description?.toLowerCase().includes(query) ?? false) ||
          rule.action_types.some((t) => t.toLowerCase().includes(query)) ||
          rule.threat_levels.some((t) => t.toLowerCase().includes(query))
      );
    }

    // Apply sorting
    result.sort((a, b) => {
      const aValue = a[sortConfig.key as keyof InterventionRule];
      const bValue = b[sortConfig.key as keyof InterventionRule];

      if (aValue === bValue) return 0;
      if (aValue === null || aValue === undefined) return 1;
      if (bValue === null || bValue === undefined) return -1;

      const comparison = aValue < bValue ? -1 : 1;
      return sortConfig.direction === 'asc' ? comparison : -comparison;
    });

    return result;
  }, [rules, filter, sortConfig]);

  // Get selected rule
  const selectedRule = useMemo(() => {
    if (!selectedRuleId) return null;
    return rules.find((r) => r.rule_id === selectedRuleId) || null;
  }, [rules, selectedRuleId]);

  // Get rule by ID
  const getRuleById = useCallback(
    (ruleId: string): InterventionRule | undefined => {
      return rules.find((r) => r.rule_id === ruleId);
    },
    [rules]
  );

  // Toggle sort
  const toggleSort = useCallback(
    (key: string) => {
      if (sortConfig.key === key) {
        setSortConfig({
          key,
          direction: sortConfig.direction === 'asc' ? 'desc' : 'asc',
        });
      } else {
        setSortConfig({ key, direction: 'asc' });
      }
    },
    [sortConfig, setSortConfig]
  );

  // Create rule
  const createRule = useCallback(
    (rule: InterventionRuleCreate) => {
      return createMutation.mutateAsync(rule);
    },
    [createMutation]
  );

  // Update rule
  const updateRule = useCallback(
    (ruleId: string, rule: InterventionRuleUpdate) => {
      return updateMutation.mutateAsync({ ruleId, rule });
    },
    [updateMutation]
  );

  // Delete rule
  const deleteRule = useCallback(
    (ruleId: string) => {
      return deleteMutation.mutateAsync(ruleId);
    },
    [deleteMutation]
  );

  // Toggle rule enabled status
  const toggleRuleEnabled = useCallback(
    (rule: InterventionRule) => {
      return updateMutation.mutateAsync({
        ruleId: rule.rule_id,
        rule: { enabled: !rule.enabled },
      });
    },
    [updateMutation]
  );

  return {
    // Data
    rules: filteredRules,
    allRules: rules,
    ruleCount: rules.length,
    selectedRule,
    selectedRuleId,

    // State
    isLoading,
    error,
    filter,
    sortConfig,
    modalOpen,
    editingRule,
    deleteConfirmOpen,
    deleteTargetId,

    // Mutation states
    isCreating: createMutation.isPending,
    isUpdating: updateMutation.isPending,
    isDeleting: deleteMutation.isPending,
    createError: createMutation.error,
    updateError: updateMutation.error,
    deleteError: deleteMutation.error,

    // Actions
    refetch,
    selectRule,
    getRuleById,
    toggleSort,
    setFilter,
    clearFilter,
    openModal,
    closeModal,
    openDeleteConfirm,
    closeDeleteConfirm,

    // CRUD operations
    createRule,
    updateRule,
    deleteRule,
    toggleRuleEnabled,
  };
}

export default useInterventionRules;
