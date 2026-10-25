import { useEffect, useRef, useCallback } from "react";

type SSEEventHandler = (data: unknown) => void;

interface UseSSEOptions {
  /** Map of event name → handler */
  events: Record<string, SSEEventHandler>;
  /** Whether to connect (default true) */
  enabled?: boolean;
  /** Map of event name → fetcher, polled when SSE never connects (e.g. buffering webviews) */
  pollFallback?: Record<string, () => Promise<unknown>>;
  /** Polling interval in ms (default 5000) */
  pollInterval?: number;
}

const OPEN_TIMEOUT = 5000;
const MAX_INITIAL_FAILURES = 3;

/**
 * Hook that maintains an EventSource (SSE) connection to /api/events.
 * Automatically reconnects on errors with exponential backoff.
 * If the stream never opens and pollFallback is given, switches to polling for good.
 */
export function useSSE({ events, enabled = true, pollFallback, pollInterval = 5000 }: UseSSEOptions) {
  const eventsRef = useRef(events);
  eventsRef.current = events;
  const pollFallbackRef = useRef(pollFallback);
  pollFallbackRef.current = pollFallback;

  const forceReconnect = useCallback(() => {
    // Trigger reconnect by dispatching a custom event
    window.dispatchEvent(new CustomEvent("sse-reconnect"));
  }, []);

  useEffect(() => {
    if (!enabled) return;

    let es: EventSource | null = null;
    let retryDelay = 1000;
    let retryTimeout: ReturnType<typeof setTimeout> | null = null;
    let disposed = false;
    let everOpened = false;
    let failures = 0;
    let polling = false;
    let openTimeout: ReturnType<typeof setTimeout> | null = null;
    let pollTimer: ReturnType<typeof setInterval> | null = null;

    function poll() {
      for (const [name, fetcher] of Object.entries(pollFallbackRef.current ?? {})) {
        fetcher()
          .then((data) => {
            if (!disposed) eventsRef.current[name]?.(data);
          })
          .catch(() => {
            // ignore fetch errors, retry on next tick
          });
      }
    }

    function startPolling() {
      if (polling || disposed) return;
      polling = true;
      es?.close();
      es = null;
      if (retryTimeout) clearTimeout(retryTimeout);
      if (openTimeout) clearTimeout(openTimeout);
      poll();
      pollTimer = setInterval(poll, pollInterval);
    }

    function connect() {
      if (disposed || polling) return;

      es = new EventSource("/api/events");

      if (!everOpened && pollFallbackRef.current) {
        if (openTimeout) clearTimeout(openTimeout);
        openTimeout = setTimeout(startPolling, OPEN_TIMEOUT);
      }

      es.onopen = () => {
        everOpened = true;
        if (openTimeout) clearTimeout(openTimeout);
        retryDelay = 1000; // reset backoff on success
      };

      // Listen for each registered event type
      const handler = (e: MessageEvent) => {
        try {
          const data = JSON.parse(e.data);
          const fn = eventsRef.current[e.type];
          if (fn) fn(data);
        } catch {
          // ignore parse errors
        }
      };

      for (const eventName of Object.keys(eventsRef.current)) {
        es.addEventListener(eventName, handler);
      }

      es.onerror = () => {
        es?.close();
        es = null;
        if (!everOpened && pollFallbackRef.current && ++failures >= MAX_INITIAL_FAILURES) {
          startPolling();
          return;
        }
        if (!disposed) {
          retryTimeout = setTimeout(connect, retryDelay);
          retryDelay = Math.min(retryDelay * 2, 30000);
        }
      };
    }

    connect();

    // Listen for forced reconnects
    const onReconnect = () => {
      if (polling) {
        poll();
        return;
      }
      es?.close();
      es = null;
      if (retryTimeout) clearTimeout(retryTimeout);
      retryDelay = 1000;
      connect();
    };
    window.addEventListener("sse-reconnect", onReconnect);

    return () => {
      disposed = true;
      window.removeEventListener("sse-reconnect", onReconnect);
      if (retryTimeout) clearTimeout(retryTimeout);
      if (openTimeout) clearTimeout(openTimeout);
      if (pollTimer) clearInterval(pollTimer);
      es?.close();
    };
  }, [enabled, pollInterval]);

  return { forceReconnect };
}
