import { useEffect, useRef, useCallback, useState } from 'react';
import type {
  WSMessage,
  ConnectionStatus,
  CorrelatedTrack,
  ActionProposal,
  Decision,
  EffectLog,
  SystemMetrics,
} from '../types';

const WS_URL = import.meta.env.VITE_WS_URL || 'ws://localhost:8080/ws';
const RECONNECT_DELAY_MS = 3000;
const MAX_RECONNECT_ATTEMPTS = 10;
const HEARTBEAT_INTERVAL_MS = 30000;

interface UseWebSocketOptions {
  onTrackUpdate?: (track: CorrelatedTrack) => void;
  onTrackDelete?: (trackId: string) => void;
  onProposalNew?: (proposal: ActionProposal) => void;
  onProposalUpdate?: (proposal: ActionProposal) => void;
  onProposalExpired?: (proposalId: string) => void;
  onDecisionMade?: (decision: Decision) => void;
  onEffectExecuted?: (effect: EffectLog) => void;
  onMetricsUpdate?: (metrics: SystemMetrics) => void;
  onConnectionChange?: (status: ConnectionStatus) => void;
}

interface UseWebSocketReturn {
  status: ConnectionStatus;
  reconnect: () => void;
  lastMessage: WSMessage | null;
  messageCount: number;
}

export function useWebSocket(options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const [status, setStatus] = useState<ConnectionStatus>('disconnected');
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null);
  const [messageCount, setMessageCount] = useState(0);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>();
  const heartbeatIntervalRef = useRef<ReturnType<typeof setInterval>>();

  const optionsRef = useRef(options);
  optionsRef.current = options;

  const updateStatus = useCallback((newStatus: ConnectionStatus) => {
    setStatus(newStatus);
    optionsRef.current.onConnectionChange?.(newStatus);
  }, []);

  const handleMessage = useCallback((event: MessageEvent) => {
    try {
      const message: WSMessage = JSON.parse(event.data);
      setLastMessage(message);
      setMessageCount((prev) => prev + 1);

      switch (message.type) {
        case 'track.update':
          optionsRef.current.onTrackUpdate?.(message.payload as CorrelatedTrack);
          break;
        case 'track.delete':
          optionsRef.current.onTrackDelete?.(message.payload as string);
          break;
        case 'proposal.new':
          optionsRef.current.onProposalNew?.(message.payload as ActionProposal);
          break;
        case 'proposal.update':
          optionsRef.current.onProposalUpdate?.(message.payload as ActionProposal);
          break;
        case 'proposal.expired':
          optionsRef.current.onProposalExpired?.(message.payload as string);
          break;
        case 'decision.made':
          optionsRef.current.onDecisionMade?.(message.payload as Decision);
          break;
        case 'effect.executed':
          optionsRef.current.onEffectExecuted?.(message.payload as EffectLog);
          break;
        case 'metrics.update':
          optionsRef.current.onMetricsUpdate?.(message.payload as SystemMetrics);
          break;
        case 'connection.status':
        case 'ping':
        case 'pong':
          // Server keepalive and acknowledgment messages, no action needed
          break;
        case 'track.new':
          // New track detected - treat same as track update
          optionsRef.current.onTrackUpdate?.(message.payload as CorrelatedTrack);
          break;
        default:
          console.warn('Unknown WebSocket message type:', message.type);
      }
    } catch (error) {
      console.error('Failed to parse WebSocket message:', error);
    }
  }, []);

  const startHeartbeat = useCallback(() => {
    if (heartbeatIntervalRef.current) {
      clearInterval(heartbeatIntervalRef.current);
    }

    heartbeatIntervalRef.current = setInterval(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ type: 'ping' }));
      }
    }, HEARTBEAT_INTERVAL_MS);
  }, []);

  const stopHeartbeat = useCallback(() => {
    if (heartbeatIntervalRef.current) {
      clearInterval(heartbeatIntervalRef.current);
      heartbeatIntervalRef.current = undefined;
    }
  }, []);

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    // Clean up existing connection
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }

    updateStatus('connecting');

    try {
      const ws = new WebSocket(WS_URL);

      ws.onopen = () => {
        console.log('WebSocket connected');
        reconnectAttemptsRef.current = 0;
        updateStatus('connected');
        startHeartbeat();
      };

      ws.onmessage = handleMessage;

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        updateStatus('error');
      };

      ws.onclose = (event) => {
        console.log('WebSocket closed:', event.code, event.reason);
        stopHeartbeat();

        if (reconnectAttemptsRef.current < MAX_RECONNECT_ATTEMPTS) {
          updateStatus('disconnected');
          const delay = RECONNECT_DELAY_MS * Math.pow(1.5, reconnectAttemptsRef.current);
          reconnectAttemptsRef.current++;

          console.log(
            `Reconnecting in ${delay}ms (attempt ${reconnectAttemptsRef.current}/${MAX_RECONNECT_ATTEMPTS})`
          );

          reconnectTimeoutRef.current = setTimeout(connect, delay);
        } else {
          updateStatus('error');
          console.error('Max reconnection attempts reached');
        }
      };

      wsRef.current = ws;
    } catch (error) {
      console.error('Failed to create WebSocket:', error);
      updateStatus('error');
    }
  }, [updateStatus, handleMessage, startHeartbeat, stopHeartbeat]);

  const reconnect = useCallback(() => {
    reconnectAttemptsRef.current = 0;
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
    }
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    connect();
  }, [connect]);

  // Connect on mount
  useEffect(() => {
    connect();

    return () => {
      stopHeartbeat();
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [connect, stopHeartbeat]);

  return {
    status,
    reconnect,
    lastMessage,
    messageCount,
  };
}

export default useWebSocket;
