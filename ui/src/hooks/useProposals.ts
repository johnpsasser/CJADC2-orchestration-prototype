import { useCallback, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { create } from 'zustand';
import { api } from '../api/client';
import type { ActionProposal, Decision, DecisionRequest } from '../types';

// Zustand store for proposal state
interface ProposalStore {
  proposals: Map<string, ActionProposal>;
  selectedProposalId: string | null;
  decisionModalOpen: boolean;
  initialDecision: 'approve' | 'deny' | null;
  setProposals: (proposals: ActionProposal[]) => void;
  addProposal: (proposal: ActionProposal) => void;
  updateProposal: (proposal: ActionProposal) => void;
  removeProposal: (proposalId: string) => void;
  selectProposal: (proposalId: string | null) => void;
  openDecisionModal: (proposalId: string, initialDecision?: 'approve' | 'deny') => void;
  closeDecisionModal: () => void;
}

export const useProposalStore = create<ProposalStore>((set) => ({
  proposals: new Map(),
  selectedProposalId: null,
  decisionModalOpen: false,
  initialDecision: null,
  setProposals: (proposals) =>
    set({
      proposals: new Map(proposals.map((p) => [p.proposal_id, p])),
    }),
  addProposal: (proposal) =>
    set((state) => {
      const newProposals = new Map(state.proposals);
      newProposals.set(proposal.proposal_id, proposal);
      return { proposals: newProposals };
    }),
  updateProposal: (proposal) =>
    set((state) => {
      const newProposals = new Map(state.proposals);
      newProposals.set(proposal.proposal_id, proposal);
      return { proposals: newProposals };
    }),
  removeProposal: (proposalId) =>
    set((state) => {
      const newProposals = new Map(state.proposals);
      newProposals.delete(proposalId);
      return {
        proposals: newProposals,
        selectedProposalId:
          state.selectedProposalId === proposalId ? null : state.selectedProposalId,
        decisionModalOpen:
          state.selectedProposalId === proposalId ? false : state.decisionModalOpen,
      };
    }),
  selectProposal: (proposalId) => set({ selectedProposalId: proposalId }),
  openDecisionModal: (proposalId, initialDecision) =>
    set({ selectedProposalId: proposalId, decisionModalOpen: true, initialDecision: initialDecision ?? null }),
  closeDecisionModal: () => set({ decisionModalOpen: false, initialDecision: null }),
}));

// Query key for proposals
const PROPOSALS_QUERY_KEY = ['proposals', 'pending'];

// Hook for fetching and managing proposals
export function useProposals() {
  const queryClient = useQueryClient();
  const {
    proposals,
    selectedProposalId,
    decisionModalOpen,
    initialDecision,
    setProposals,
    addProposal,
    updateProposal,
    removeProposal,
    selectProposal,
    openDecisionModal,
    closeDecisionModal,
  } = useProposalStore();

  // Fetch pending proposals from API
  const {
    data,
    isLoading,
    error,
    refetch,
  } = useQuery({
    queryKey: PROPOSALS_QUERY_KEY,
    queryFn: async () => {
      const response = await api.proposals.getPending();
      setProposals(response.data);
      return response.data;
    },
    refetchInterval: 10000, // Refetch every 10 seconds
    staleTime: 5000,
  });

  // Submit decision mutation
  const decisionMutation = useMutation({
    mutationFn: async (request: DecisionRequest) => {
      const response = await api.proposals.submitDecision(request);
      return response.data;
    },
    onSuccess: (decision: Decision) => {
      // Remove the proposal from the pending list
      removeProposal(decision.proposal_id);
      queryClient.setQueryData<ActionProposal[]>(PROPOSALS_QUERY_KEY, (old) => {
        if (!old) return [];
        return old.filter((p) => p.proposal_id !== decision.proposal_id);
      });
      closeDecisionModal();
    },
    onError: (error: Error & { code?: string }, variables: DecisionRequest) => {
      // 409 Conflict means already decided - remove from list anyway
      if (error.code === 'HTTP_409' || error.message?.includes('conflict')) {
        removeProposal(variables.proposal_id);
        queryClient.setQueryData<ActionProposal[]>(PROPOSALS_QUERY_KEY, (old) => {
          if (!old) return [];
          return old.filter((p) => p.proposal_id !== variables.proposal_id);
        });
        closeDecisionModal();
      }
    },
  });

  // Handle WebSocket new proposal - refetch from API after delay to ensure DB has it
  const handleProposalNew = useCallback(
    (_proposal: ActionProposal) => {
      // Wait for authorizer to store in DB, then refetch
      setTimeout(() => {
        refetch();
      }, 500);
    },
    [refetch]
  );

  // Handle WebSocket proposal update
  const handleProposalUpdate = useCallback(
    (proposal: ActionProposal) => {
      updateProposal(proposal);
      queryClient.setQueryData<ActionProposal[]>(PROPOSALS_QUERY_KEY, (old) => {
        if (!old) return [proposal];
        const index = old.findIndex((p) => p.proposal_id === proposal.proposal_id);
        if (index >= 0) {
          const newProposals = [...old];
          newProposals[index] = proposal;
          return newProposals;
        }
        return [...old, proposal];
      });
    },
    [updateProposal, queryClient]
  );

  // Handle WebSocket proposal expired
  const handleProposalExpired = useCallback(
    (proposalId: string) => {
      removeProposal(proposalId);
      queryClient.setQueryData<ActionProposal[]>(PROPOSALS_QUERY_KEY, (old) => {
        if (!old) return [];
        return old.filter((p) => p.proposal_id !== proposalId);
      });
    },
    [removeProposal, queryClient]
  );

  // Get sorted proposals (by priority, then expiration), excluding expired
  const sortedProposals = useMemo(() => {
    const now = Date.now();
    return Array.from(proposals.values())
      .filter((p) => new Date(p.expires_at).getTime() > now) // Exclude expired
      .sort((a, b) => {
        // Higher priority first
        if (a.priority !== b.priority) {
          return b.priority - a.priority;
        }
        // Earlier expiration first
        return new Date(a.expires_at).getTime() - new Date(b.expires_at).getTime();
      });
  }, [proposals]);

  // Get selected proposal
  const selectedProposal = useMemo(() => {
    if (!selectedProposalId) return null;
    return proposals.get(selectedProposalId) || null;
  }, [proposals, selectedProposalId]);

  // Get proposal by ID
  const getProposalById = useCallback(
    (proposalId: string): ActionProposal | undefined => {
      return proposals.get(proposalId);
    },
    [proposals]
  );

  // Submit approval
  const approve = useCallback(
    (proposalId: string, reason: string, conditions?: string[]) => {
      return decisionMutation.mutateAsync({
        proposal_id: proposalId,
        approved: true,
        reason,
        conditions,
      });
    },
    [decisionMutation]
  );

  // Submit denial
  const deny = useCallback(
    (proposalId: string, reason: string) => {
      return decisionMutation.mutateAsync({
        proposal_id: proposalId,
        approved: false,
        reason,
      });
    },
    [decisionMutation]
  );

  // Get proposals by priority level
  const getProposalsByPriority = useCallback(
    (minPriority: number): ActionProposal[] => {
      return sortedProposals.filter((p) => p.priority >= minPriority);
    },
    [sortedProposals]
  );

  // Get expiring soon proposals (within 60 seconds)
  const expiringSoonProposals = useMemo(() => {
    const now = Date.now();
    const threshold = 60 * 1000; // 60 seconds
    return sortedProposals.filter((p) => {
      const expiresAt = new Date(p.expires_at).getTime();
      return expiresAt - now <= threshold && expiresAt > now;
    });
  }, [sortedProposals]);

  return {
    // Data
    proposals: sortedProposals,
    proposalCount: proposals.size,
    selectedProposal,
    selectedProposalId,
    expiringSoonProposals,

    // State
    isLoading,
    error,
    isSubmitting: decisionMutation.isPending,
    submitError: decisionMutation.error,
    decisionModalOpen,
    initialDecision,

    // Actions
    refetch,
    selectProposal,
    getProposalById,
    getProposalsByPriority,
    approve,
    deny,
    openDecisionModal,
    closeDecisionModal,

    // WebSocket handlers
    handleProposalNew,
    handleProposalUpdate,
    handleProposalExpired,
  };
}

export default useProposals;
