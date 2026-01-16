import { useState, useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  AreaChart,
  Area,
  BarChart,
  Bar,
} from 'recharts';
import { format } from 'date-fns';
import clsx from 'clsx';
import { api } from '../api/client';
import type { SystemMetrics, StageMetrics } from '../types';

interface MetricsDashboardProps {
  wsMetrics?: SystemMetrics | null;
}

// Stage display names
const stageNames: Record<string, string> = {
  sensor: 'Sensor',
  classifier: 'Classifier',
  correlator: 'Correlator',
  planner: 'Planner',
  authorizer: 'Authorizer',
  effector: 'Effector',
};

// Stage colors for charts
const stageColors: Record<string, string> = {
  sensor: '#22c55e',
  classifier: '#3b82f6',
  correlator: '#a855f7',
  planner: '#f59e0b',
  authorizer: '#06b6d4',
  effector: '#ef4444',
};

// Metric card component
interface MetricCardProps {
  label: string;
  value: string | number;
  unit?: string;
  trend?: 'up' | 'down' | 'stable';
  color?: string;
}

function MetricCard({ label, value, unit, trend, color = 'text-green-400' }: MetricCardProps) {
  return (
    <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
      <p className="text-xs font-medium text-gray-500 uppercase tracking-wide">{label}</p>
      <div className="mt-2 flex items-baseline gap-1">
        <span className={clsx('text-2xl font-bold', color)}>{value}</span>
        {unit && <span className="text-sm text-gray-500">{unit}</span>}
        {trend && (
          <svg
            className={clsx(
              'w-4 h-4 ml-2',
              trend === 'up' ? 'text-green-400' : trend === 'down' ? 'text-red-400' : 'text-gray-500'
            )}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            {trend === 'up' && (
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 15l7-7 7 7" />
            )}
            {trend === 'down' && (
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            )}
            {trend === 'stable' && (
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 12h14" />
            )}
          </svg>
        )}
      </div>
    </div>
  );
}

// Stage metrics card component
function StageMetricsCard({ stage }: { stage: StageMetrics }) {
  const successRate = stage.processed > 0 ? (stage.succeeded / stage.processed) * 100 : 100;
  const color = stageColors[stage.stage] || '#6b7280';

  return (
    <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-medium text-gray-300" style={{ color }}>
          {stageNames[stage.stage] || stage.stage}
        </h3>
        <span
          className={clsx(
            'text-xs font-medium px-2 py-0.5 rounded',
            successRate >= 99
              ? 'bg-green-900/40 text-green-400'
              : successRate >= 95
              ? 'bg-yellow-900/40 text-yellow-400'
              : 'bg-red-900/40 text-red-400'
          )}
        >
          {successRate.toFixed(1)}%
        </span>
      </div>

      <div className="grid grid-cols-3 gap-3 text-center">
        <div>
          <p className="text-lg font-bold text-gray-200">{stage.processed}</p>
          <p className="text-xs text-gray-500">Processed</p>
        </div>
        <div>
          <p className="text-lg font-bold text-green-400">{stage.succeeded}</p>
          <p className="text-xs text-gray-500">Succeeded</p>
        </div>
        <div>
          <p className="text-lg font-bold text-red-400">{stage.failed}</p>
          <p className="text-xs text-gray-500">Failed</p>
        </div>
      </div>

      <div className="mt-3 pt-3 border-t border-gray-700">
        <div className="flex justify-between text-xs">
          <span className="text-gray-500">Latency</span>
          <div className="flex gap-3">
            <span className="text-gray-400">
              p50: <span className="text-gray-300">{stage.latency_p50_ms}ms</span>
            </span>
            <span className="text-gray-400">
              p95: <span className="text-gray-300">{stage.latency_p95_ms}ms</span>
            </span>
            <span className="text-gray-400">
              p99: <span className="text-gray-300">{stage.latency_p99_ms}ms</span>
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}

// Custom tooltip for charts
function CustomTooltip({ active, payload, label }: any) {
  if (active && payload && payload.length) {
    return (
      <div className="bg-gray-900 border border-gray-700 rounded p-3 shadow-lg">
        <p className="text-xs text-gray-400 mb-2">{label}</p>
        {payload.map((entry: any, index: number) => (
          <p key={index} className="text-sm" style={{ color: entry.color }}>
            {entry.name}: <span className="font-medium">{entry.value}ms</span>
          </p>
        ))}
      </div>
    );
  }
  return null;
}

export function MetricsDashboard({ wsMetrics }: MetricsDashboardProps) {
  // State for historical data
  const [latencyHistory, setLatencyHistory] = useState<
    Array<{ timestamp: string; p50: number; p95: number; p99: number }>
  >([]);

  // Fetch metrics from API
  const { data: apiMetrics, isLoading } = useQuery({
    queryKey: ['metrics'],
    queryFn: async () => {
      const response = await api.metrics.getCurrent();
      return response.data;
    },
    refetchInterval: 5000,
    staleTime: 3000,
  });

  // Use WebSocket metrics if available, fallback to API
  const metrics = wsMetrics || apiMetrics;

  // Update latency history when metrics change
  useEffect(() => {
    if (metrics) {
      setLatencyHistory((prev) => {
        const newPoint = {
          timestamp: format(new Date(metrics.timestamp), 'HH:mm:ss'),
          p50: metrics.end_to_end_latency_p50_ms,
          p95: metrics.end_to_end_latency_p95_ms,
          p99: metrics.end_to_end_latency_p99_ms,
        };

        // Keep last 60 points (5 minutes at 5s intervals)
        const updated = [...prev, newPoint].slice(-60);
        return updated;
      });
    }
  }, [metrics?.timestamp]);

  // Mock throughput data for demo (in production, this would come from the API)
  const throughputData = useMemo(() => {
    return Array.from({ length: 12 }, (_, i) => ({
      time: format(new Date(Date.now() - (11 - i) * 5000), 'HH:mm:ss'),
      messages: Math.floor(Math.random() * 50) + (metrics?.messages_per_minute || 100) / 12,
    }));
  }, [metrics?.messages_per_minute]);

  if (isLoading && !metrics) {
    return (
      <div className="flex items-center justify-center h-64 bg-gray-900 rounded-lg border border-gray-700">
        <div className="flex flex-col items-center gap-3">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-green-500" />
          <span className="text-gray-400 text-sm">Loading metrics...</span>
        </div>
      </div>
    );
  }

  if (!metrics) {
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
          <h3 className="mt-2 text-sm font-medium text-gray-400">No metrics available</h3>
          <p className="mt-1 text-sm text-gray-500">Waiting for system data...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Top-level metrics */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <MetricCard
          label="Messages/Min"
          value={metrics.messages_per_minute}
          color="text-blue-400"
        />
        <MetricCard
          label="Active Tracks"
          value={metrics.active_tracks}
          color="text-green-400"
        />
        <MetricCard
          label="Pending Proposals"
          value={metrics.pending_proposals}
          color={metrics.pending_proposals > 0 ? 'text-yellow-400' : 'text-gray-400'}
        />
        <MetricCard
          label="E2E Latency (p95)"
          value={
            metrics.end_to_end_latency_p95_ms >= 1000
              ? (metrics.end_to_end_latency_p95_ms / 1000).toFixed(2)
              : metrics.end_to_end_latency_p95_ms.toFixed(1)
          }
          unit={metrics.end_to_end_latency_p95_ms >= 1000 ? 's' : 'ms'}
          color={
            metrics.end_to_end_latency_p95_ms < 100
              ? 'text-green-400'
              : metrics.end_to_end_latency_p95_ms < 500
              ? 'text-yellow-400'
              : 'text-red-400'
          }
        />
      </div>

      {/* Charts Row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* End-to-End Latency Chart */}
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
          <h3 className="text-sm font-medium text-gray-300 mb-4">End-to-End Latency</h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={latencyHistory}>
                <defs>
                  <linearGradient id="p50Gradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#22c55e" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="#22c55e" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="p95Gradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="p99Gradient" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#ef4444" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="#ef4444" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                <XAxis
                  dataKey="timestamp"
                  stroke="#6b7280"
                  tick={{ fill: '#9ca3af', fontSize: 10 }}
                />
                <YAxis
                  stroke="#6b7280"
                  tick={{ fill: '#9ca3af', fontSize: 10 }}
                  unit="ms"
                />
                <Tooltip content={<CustomTooltip />} />
                <Area
                  type="monotone"
                  dataKey="p50"
                  name="p50"
                  stroke="#22c55e"
                  fill="url(#p50Gradient)"
                  strokeWidth={2}
                />
                <Area
                  type="monotone"
                  dataKey="p95"
                  name="p95"
                  stroke="#f59e0b"
                  fill="url(#p95Gradient)"
                  strokeWidth={2}
                />
                <Area
                  type="monotone"
                  dataKey="p99"
                  name="p99"
                  stroke="#ef4444"
                  fill="url(#p99Gradient)"
                  strokeWidth={2}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
          <div className="flex justify-center gap-6 mt-2">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-green-500 rounded-full" />
              <span className="text-xs text-gray-400">p50</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-yellow-500 rounded-full" />
              <span className="text-xs text-gray-400">p95</span>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-red-500 rounded-full" />
              <span className="text-xs text-gray-400">p99</span>
            </div>
          </div>
        </div>

        {/* Throughput Chart */}
        <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
          <h3 className="text-sm font-medium text-gray-300 mb-4">Message Throughput</h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={throughputData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#374151" />
                <XAxis
                  dataKey="time"
                  stroke="#6b7280"
                  tick={{ fill: '#9ca3af', fontSize: 10 }}
                />
                <YAxis stroke="#6b7280" tick={{ fill: '#9ca3af', fontSize: 10 }} />
                <Tooltip
                  contentStyle={{
                    backgroundColor: '#1f2937',
                    border: '1px solid #374151',
                    borderRadius: '8px',
                  }}
                  labelStyle={{ color: '#9ca3af' }}
                />
                <Bar dataKey="messages" fill="#3b82f6" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      </div>

      {/* Stage Metrics */}
      <div>
        <h3 className="text-sm font-medium text-gray-300 mb-4">Pipeline Stages</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {metrics.stages.map((stage) => (
            <StageMetricsCard key={stage.stage} stage={stage} />
          ))}
        </div>
      </div>

      {/* Pipeline Flow Visualization */}
      <div className="bg-gray-800 rounded-lg p-4 border border-gray-700">
        <h3 className="text-sm font-medium text-gray-300 mb-4">Pipeline Flow</h3>
        <div className="flex items-center justify-between overflow-x-auto pb-2">
          {['sensor', 'classifier', 'correlator', 'planner', 'authorizer', 'effector'].map(
            (stageName, index, arr) => {
              const stage = metrics.stages.find((s) => s.stage === stageName);
              const successRate = stage && stage.processed > 0
                ? (stage.succeeded / stage.processed) * 100
                : 100;

              return (
                <div key={stageName} className="flex items-center">
                  <div
                    className="flex flex-col items-center px-4 py-3 rounded-lg bg-gray-900 border min-w-[100px]"
                    style={{ borderColor: stageColors[stageName] }}
                  >
                    <span
                      className="text-xs font-medium uppercase"
                      style={{ color: stageColors[stageName] }}
                    >
                      {stageNames[stageName]}
                    </span>
                    <span className="text-lg font-bold text-gray-200 mt-1">
                      {stage?.processed || 0}
                    </span>
                    <span
                      className={clsx(
                        'text-xs mt-1',
                        successRate >= 99
                          ? 'text-green-400'
                          : successRate >= 95
                          ? 'text-yellow-400'
                          : 'text-red-400'
                      )}
                    >
                      {successRate.toFixed(0)}%
                    </span>
                  </div>
                  {index < arr.length - 1 && (
                    <svg
                      className="w-8 h-8 text-gray-600 mx-2 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M14 5l7 7m0 0l-7 7m7-7H3"
                      />
                    </svg>
                  )}
                </div>
              );
            }
          )}
        </div>
      </div>
    </div>
  );
}

export default MetricsDashboard;
