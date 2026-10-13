import { useEffect, useRef, useState, useCallback } from 'react';
import type { WSMessage } from '../types';
import { getToken } from '../services/api';

interface UseWebSocketOptions {
  onMessage?: (message: WSMessage) => void;
  /** Reconnect only while enabled (e.g. after login). Default true. */
  enabled?: boolean;
}

const RECONNECT_DELAY_MS = 5000;

export function useWebSocket(options?: UseWebSocketOptions) {
  const [isConnected, setIsConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>();
  const unmountedRef = useRef(false);
  const onMessageRef = useRef(options?.onMessage);
  const enabled = options?.enabled ?? true;

  onMessageRef.current = options?.onMessage;

  const scheduleReconnect = useCallback((connect: () => void) => {
    if (unmountedRef.current) return;
    if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
    reconnectTimeoutRef.current = setTimeout(connect, RECONNECT_DELAY_MS);
  }, []);

  const connect = useCallback(() => {
    if (unmountedRef.current) return;
    if (wsRef.current && wsRef.current.readyState <= WebSocket.OPEN) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const token = getToken();
    const wsUrl = `${protocol}//${window.location.host}/ws${token ? `?token=${encodeURIComponent(token)}` : ''}`;

    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        if (unmountedRef.current) return;
        setIsConnected(true);
      };

      ws.onclose = () => {
        if (wsRef.current === ws) wsRef.current = null;
        if (unmountedRef.current) return;
        setIsConnected(false);
        scheduleReconnect(connect);
      };

      ws.onerror = () => {
        // onclose follows and handles the reconnect
      };

      ws.onmessage = (event) => {
        try {
          const message: WSMessage = JSON.parse(event.data);
          onMessageRef.current?.(message);
        } catch (err) {
          console.error('[WebSocket] Nachricht konnte nicht geparst werden:', err);
        }
      };
    } catch (err) {
      console.error('[WebSocket] Verbindungsfehler:', err);
      scheduleReconnect(connect);
    }
  }, [scheduleReconnect]);

  const sendCommand = useCallback(
    (target: string, action: string, params?: Record<string, unknown>) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(
          JSON.stringify({
            type: 'command',
            payload: { type: 'command', target, action, params },
          })
        );
      }
    },
    []
  );

  useEffect(() => {
    unmountedRef.current = false;
    if (enabled) connect();

    return () => {
      unmountedRef.current = true;
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
        reconnectTimeoutRef.current = undefined;
      }
      if (wsRef.current) {
        const ws = wsRef.current;
        wsRef.current = null;
        ws.onclose = null;
        ws.close();
      }
      setIsConnected(false);
    };
  }, [connect, enabled]);

  return { isConnected, sendCommand };
}
