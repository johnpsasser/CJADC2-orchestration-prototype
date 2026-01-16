import { useState, useCallback, useMemo } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import clsx from 'clsx';

import { useWebSocket } from './hooks/useWebSocket';
import { useTracks } from './hooks/useTracks';
import { useProposals } from './hooks/useProposals';
import { TrackTable, TrackDetail } from './components/TrackTable';
import { ProposalQueue } from './components/ProposalQueue';
import { DecisionModal } from './components/DecisionModal';
import { MetricsDashboard } from './components/MetricsDashboard';
import { AuditTrail } from './components/AuditTrail';
import type { ConnectionStatus, SystemMetrics } from './types';

// Create a client
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 3,
      retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 30000),
    },
  },
});

// Tab type
type TabId = 'tracks' | 'proposals' | 'metrics' | 'audit';

// Connection status indicator
function ConnectionIndicator({ status, onReconnect }: { status: ConnectionStatus; onReconnect: () => void }) {
  const statusConfig = {
    connected: { color: 'bg-green-500', text: 'Connected', pulse: false },
    connecting: { color: 'bg-yellow-500', text: 'Connecting...', pulse: true },
    disconnected: { color: 'bg-red-500', text: 'Disconnected', pulse: false },
    error: { color: 'bg-red-500', text: 'Error', pulse: false },
  };

  const config = statusConfig[status];

  return (
    <div className="flex items-center gap-2">
      <div className="flex items-center gap-2">
        <div className={clsx('w-2 h-2 rounded-full', config.color, config.pulse && 'animate-pulse')} />
        <span className="text-xs text-gray-400">{config.text}</span>
      </div>
      {(status === 'disconnected' || status === 'error') && (
        <button
          onClick={onReconnect}
          className="text-xs text-green-400 hover:text-green-300 underline"
        >
          Reconnect
        </button>
      )}
    </div>
  );
}

// Tab navigation
function TabNavigation({
  activeTab,
  onTabChange,
  proposalCount,
  trackCount,
}: {
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
  proposalCount: number;
  trackCount: number;
}) {
  const tabs: { id: TabId; label: string; icon: string }[] = [
    {
      id: 'tracks',
      label: 'Tracks',
      icon: 'M9 20l-5.447-2.724A1 1 0 013 16.382V5.618a1 1 0 011.447-.894L9 7m0 13l6-3m-6 3V7m6 10l4.553 2.276A1 1 0 0021 18.382V7.618a1 1 0 00-.553-.894L15 4m0 13V4m0 0L9 7',
    },
    {
      id: 'proposals',
      label: 'Proposals',
      icon: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4',
    },
    {
      id: 'metrics',
      label: 'Metrics',
      icon: 'M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z',
    },
    {
      id: 'audit',
      label: 'Audit Trail',
      icon: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2',
    },
  ];

  return (
    <nav className="flex gap-1 bg-gray-800/50 p-1 rounded-lg">
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onTabChange(tab.id)}
          className={clsx(
            'flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors',
            activeTab === tab.id
              ? 'bg-gray-700 text-green-400'
              : 'text-gray-400 hover:text-gray-200 hover:bg-gray-700/50'
          )}
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={tab.icon} />
          </svg>
          {tab.label}
          {tab.id === 'tracks' && trackCount > 0 && (
            <span className="px-1.5 py-0.5 text-xs bg-green-500 text-gray-900 rounded-full font-bold">
              {trackCount}
            </span>
          )}
          {tab.id === 'proposals' && proposalCount > 0 && (
            <span className="px-1.5 py-0.5 text-xs bg-yellow-500 text-gray-900 rounded-full font-bold">
              {proposalCount}
            </span>
          )}
        </button>
      ))}
    </nav>
  );
}

// Main dashboard content
function DashboardContent() {
  const [activeTab, setActiveTab] = useState<TabId>('tracks');
  const [wsMetrics, setWsMetrics] = useState<SystemMetrics | null>(null);

  // Track hooks
  const {
    tracks,
    trackCount,
    selectedTrack,
    selectedTrackId,
    isLoading: tracksLoading,
    sortConfig,
    selectTrack,
    toggleSort,
    handleTrackUpdate,
    handleTrackDelete,
  } = useTracks();

  // Proposal hooks
  const {
    proposals,
    proposalCount,
    selectedProposal,
    isLoading: proposalsLoading,
    isSubmitting,
    decisionModalOpen,
    initialDecision,
    approve,
    deny,
    openDecisionModal,
    closeDecisionModal,
    handleProposalNew,
    handleProposalUpdate,
    handleProposalExpired,
  } = useProposals();

  // WebSocket connection
  const { status, reconnect, messageCount } = useWebSocket({
    onTrackUpdate: handleTrackUpdate,
    onTrackDelete: handleTrackDelete,
    onProposalNew: handleProposalNew,
    onProposalUpdate: handleProposalUpdate,
    onProposalExpired: handleProposalExpired,
    onMetricsUpdate: setWsMetrics,
  });

  // Handle proposal actions
  const handleApprove = useCallback((proposalId: string) => {
    openDecisionModal(proposalId, 'approve');
  }, [openDecisionModal]);

  const handleDeny = useCallback((proposalId: string) => {
    openDecisionModal(proposalId, 'deny');
  }, [openDecisionModal]);

  const handleDecisionApprove = useCallback(
    async (reason: string, conditions?: string[]) => {
      if (selectedProposal) {
        await approve(selectedProposal.proposal_id, reason, conditions);
      }
    },
    [selectedProposal, approve]
  );

  const handleDecisionDeny = useCallback(
    async (reason: string) => {
      if (selectedProposal) {
        await deny(selectedProposal.proposal_id, reason);
      }
    },
    [selectedProposal, deny]
  );

  // Render active tab content
  const renderContent = () => {
    switch (activeTab) {
      case 'tracks':
        return (
          <div className="grid grid-cols-1 lg:grid-cols-4 gap-6">
            <div className={selectedTrack ? 'lg:col-span-3' : 'lg:col-span-4'}>
              <TrackTable
                tracks={tracks}
                selectedTrackId={selectedTrackId}
                sortConfig={sortConfig}
                onSelectTrack={selectTrack}
                onSort={toggleSort}
                isLoading={tracksLoading}
              />
            </div>
            {selectedTrack && (
              <div className="lg:col-span-1">
                <TrackDetail track={selectedTrack} onClose={() => selectTrack(null)} />
              </div>
            )}
          </div>
        );

      case 'proposals':
        return (
          <ProposalQueue
            proposals={proposals}
            onApprove={handleApprove}
            onDeny={handleDeny}
            onViewDetails={openDecisionModal}
            isLoading={proposalsLoading}
          />
        );

      case 'metrics':
        return <MetricsDashboard wsMetrics={wsMetrics} />;

      case 'audit':
        return <AuditTrail />;

      default:
        return null;
    }
  };

  return (
    <div className="min-h-screen bg-gray-950">
      {/* Header */}
      <header className="bg-gray-900 border-b border-gray-800 sticky top-0 z-40">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            {/* Logo/Title */}
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 bg-green-600 rounded flex items-center justify-center">
                <svg
                  className="w-5 h-5 text-white"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
                  />
                </svg>
              </div>
              <div>
                <h1 className="text-lg font-bold text-gray-100">CJADC2</h1>
                <p className="text-xs text-gray-500">Command & Control Dashboard</p>
              </div>
            </div>

            {/* Status indicators */}
            <div className="flex items-center gap-6">
              {/* Message count */}
              <div className="flex items-center gap-2 text-xs">
                <span className="text-gray-500">Messages:</span>
                <span className="text-gray-300 font-mono">{messageCount}</span>
              </div>

              {/* Real-time indicator */}
              {status === 'connected' && (
                <div className="flex items-center gap-1 text-xs">
                  <div className="w-1.5 h-1.5 bg-green-500 rounded-full animate-pulse" />
                  <span className="text-green-400">LIVE</span>
                </div>
              )}

              {/* Connection status */}
              <ConnectionIndicator status={status} onReconnect={reconnect} />
            </div>
          </div>
        </div>
      </header>

      {/* Navigation */}
      <div className="bg-gray-900/50 border-b border-gray-800">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-3">
          <TabNavigation
            activeTab={activeTab}
            onTabChange={setActiveTab}
            proposalCount={proposalCount}
            trackCount={trackCount}
          />
        </div>
      </div>

      {/* Main content */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        {renderContent()}
      </main>

      {/* Decision Modal */}
      <DecisionModal
        proposal={selectedProposal}
        isOpen={decisionModalOpen}
        isSubmitting={isSubmitting}
        initialDecision={initialDecision}
        onApprove={handleDecisionApprove}
        onDeny={handleDecisionDeny}
        onClose={closeDecisionModal}
      />

      {/* Footer */}
      <footer className="bg-gray-900 border-t border-gray-800 mt-auto">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
          <div className="flex items-center justify-between text-xs text-gray-500">
            <span>CJADC2 Platform v1.0.0</span>
            <span>
              Last updated:{' '}
              {new Date().toLocaleTimeString('en-US', {
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit',
              })}
            </span>
          </div>
        </div>
      </footer>
    </div>
  );
}

// Main App with providers
export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <DashboardContent />
    </QueryClientProvider>
  );
}

export default App;
