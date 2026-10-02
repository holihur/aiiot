import { useEffect, useRef, useState } from "react";
import { getToken } from "./api";

export interface StreamEvent {
  type: string; // telemetry | lifecycle
  projectId: number;
  deviceId: number;
  deviceKey: string;
  identifier?: string;
  value?: unknown;
  online?: boolean;
  time: string;
}

// useEventStream subscribes to the core's SSE endpoint and returns the most
// recent events. The callback fires per event without causing reconnects.
export function useEventStream(projectId?: number, onEvent?: (e: StreamEvent) => void) {
  const [events, setEvents] = useState<StreamEvent[]>([]);
  const cb = useRef(onEvent);
  cb.current = onEvent;
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    const token = getToken();
    if (!token) return;
    const qs = new URLSearchParams({ token });
    if (projectId) qs.set("projectId", String(projectId));
    const es = new EventSource(`/api/v1/events/stream?${qs.toString()}`);
    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    es.onmessage = (m) => {
      try {
        const e = JSON.parse(m.data) as StreamEvent;
        setEvents((prev) => [e, ...prev].slice(0, 50));
        cb.current?.(e);
      } catch {
        /* ignore malformed frames */
      }
    };
    return () => {
      es.close();
      setConnected(false);
    };
  }, [projectId]);

  return { events, connected };
}
