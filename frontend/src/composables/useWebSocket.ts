import { ref, onUnmounted } from 'vue';

interface WebSocketOptions {
  onConnected?: (ws: WebSocket) => void;
  onMessage?: (event: MessageEvent) => void;
  onError?: (event: Event) => void;
  onDisconnected?: (event: CloseEvent) => void;
  autoReconnect?: boolean;
}

export function useWebSocket(url: string, options: WebSocketOptions = {}) {
  const ws = ref<WebSocket | null>(null);
  const isConnected = ref(false);
  let reconnectTimer: number | null = null;

  const connect = () => {
    ws.value = new WebSocket(url);

    ws.value.onopen = () => {
      isConnected.value = true;
      options.onConnected?.(ws.value!);
      if (reconnectTimer) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    };

    ws.value.onmessage = (event) => {
      options.onMessage?.(event);
    };

    ws.value.onerror = (event) => {
      console.error('WebSocket Error:', event);
      options.onError?.(event);
    };

    ws.value.onclose = (event) => {
      isConnected.value = false;
      options.onDisconnected?.(event);
      if (options.autoReconnect) {
        reconnectTimer = window.setTimeout(connect, 5000);
      }
    };
  };

  const disconnect = () => {
    if (ws.value) {
      ws.value.close();
    }
    if (reconnectTimer) {
      window.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  onUnmounted(() => {
    disconnect();
  });

  return {
    ws,
    isConnected,
    connect,
    disconnect,
  };
}
