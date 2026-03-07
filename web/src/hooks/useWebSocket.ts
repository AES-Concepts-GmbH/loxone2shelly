import { useEffect, useRef, useState, useCallback } from 'react';
import type { WSMessage } from '../types';

interface UseWebSocketOptions {
  onMessage?: (message: WSMessage) => void;
}

export function useWebSocket(options?: UseWebSocketOptions) {
  const [isConnected, setIsConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>();
  const onMessageRef = useRef(options?.onMessage);

  onMessageRef.current = options?.onMessage;

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    try {
      const ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        setIsConnected(true);
        console.log('[WebSocket] Verbunden');
      };

      ws.onclose = () => {
        setIsConnected(false);
        console.log('[WebSocket] Verbindung getrennt, reconnect in 5s...');
        reconnectTimeoutRef.current = setTimeout(connect, 5000);
      };

      ws.onerror = (event) => {
        console.error('[WebSocket] Fehler:', event);
      };

      ws.onmessage = (event) => {
        try {
          const message: WSMessage = JSON.parse(event.data);
          onMessageRef.current?.(message);
        } catch (err) {
          console.error('[WebSocket] Nachricht konnte nicht geparst werden:', err);
        }
      };

      wsRef.current = ws;
    } catch (err) {
      console.error('[WebSocket] Verbindungsfehler:', err);
      reconnectTimeoutRef.current = setTimeout(connect, 5000);
    }
  }, []);

  const sendCommand = useCallback(
    (target: string, action: string, params?: Record<string, unknown>) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(
          JSON.stringify({
            type: 'command',
            payload: { target, action, params },
          })
        );
      }
    },
    []
  );

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  return { isConnected, sendCommand };
}
