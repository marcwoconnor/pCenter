import { useEffect, useRef, useState } from 'react';

export interface WSMessage<T = unknown> {
  type: string;
  payload: T;
}

export interface UseWebSocketOptions {
  onMessage?: (message: WSMessage) => void;
  onConnect?: () => void;
  onDisconnect?: () => void;
  reconnectInterval?: number;
}

export function useWebSocket(url: string, options: UseWebSocketOptions = {}) {
  const { reconnectInterval = 3000 } = options;

  const [isConnected, setIsConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<WSMessage | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Store callbacks in refs to avoid re-triggering effects
  const onMessageRef = useRef(options.onMessage);
  const onConnectRef = useRef(options.onConnect);
  const onDisconnectRef = useRef(options.onDisconnect);

  // Update refs when callbacks change
  useEffect(() => {
    onMessageRef.current = options.onMessage;
    onConnectRef.current = options.onConnect;
    onDisconnectRef.current = options.onDisconnect;
  });

  useEffect(() => {
    let isMounted = true;

    const connect = () => {
      if (wsRef.current?.readyState === WebSocket.OPEN) return;
      if (wsRef.current?.readyState === WebSocket.CONNECTING) return;

      const ws = new WebSocket(url);

      ws.onopen = () => {
        if (!isMounted) return;
        setIsConnected(true);
        onConnectRef.current?.();
      };

      ws.onclose = () => {
        if (!isMounted) return;
        setIsConnected(false);
        onDisconnectRef.current?.();
        wsRef.current = null;
        // Auto-reconnect
        reconnectTimeoutRef.current = setTimeout(connect, reconnectInterval);
      };

      ws.onerror = () => {
        // onclose will be called after onerror
      };

      ws.onmessage = (event) => {
        if (!isMounted) return;
        try {
          const message = JSON.parse(event.data) as WSMessage;
          setLastMessage(message);
          onMessageRef.current?.(message);
        } catch (e) {
          console.error('Failed to parse WebSocket message:', e);
        }
      };

      wsRef.current = ws;
    };

    connect();

    return () => {
      isMounted = false;
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.onclose = null; // Prevent reconnect on unmount
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [url, reconnectInterval]);

  return { isConnected, lastMessage };
}
