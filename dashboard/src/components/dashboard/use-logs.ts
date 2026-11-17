"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { LogEntry, LogsResponse } from "@/lib/live-data";

export type LogFilter = {
  service?: string;
  level?: string;
};

type LogsState = {
  entries: LogEntry[];
  loading: boolean;
  error: string | null;
  stale: boolean;
  lastUpdated: string | null;
  hasMore: boolean;
  refresh: () => void;
};

const pollIntervalMs = 2000;
const backoffMs = [1000, 2000, 4000, 8000, 16000, 30000];
const maxEntries = 2000;

export function useLogs(filter: LogFilter): LogsState {
  const [entries, setEntries] = useState<LogEntry[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [refreshTick, setRefreshTick] = useState(0);
  const [loading, setLoading] = useState(true);
  const inFlight = useRef<AbortController | null>(null);
  const failures = useRef(0);

  const refresh = useCallback(() => {
    failures.current = 0;
    setRefreshTick((tick) => tick + 1);
  }, []);

  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let cursor: string | null = null;

    failures.current = 0;

    async function load() {
      if (stopped) {
        return;
      }
      const controller = new AbortController();
      inFlight.current?.abort();
      inFlight.current = controller;
      const params = new URLSearchParams({ limit: "200" });
      if (filter.service) {
        params.set("service", filter.service);
      }
      if (filter.level) {
        params.set("level", filter.level);
      }
      if (cursor) {
        params.set("cursor", cursor);
      }
      try {
        const response = await fetch(`/api/logs?${params.toString()}`, {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const data = (await response.json()) as LogsResponse;
        if (stopped) {
          return;
        }
        const isInitial = cursor === null;
        cursor = data.nextCursor;
        setHasMore(data.hasMore);
        setLastUpdated(data.generatedAt ?? new Date().toISOString());
        setError(
          data.sources.docker.state === "unavailable"
            ? (data.sources.docker.message ?? "Docker API is not available.")
            : null,
        );
        failures.current = 0;
        setEntries((current) => {
          const merged = isInitial ? data.entries : [...current, ...data.entries];
          const seen = new Set<string>();
          const deduplicated = merged.filter((entry) => {
            if (seen.has(entry.id)) {
              return false;
            }
            seen.add(entry.id);
            return true;
          });
          return deduplicated.slice(-maxEntries);
        });
      } catch (requestError) {
        if (stopped || (requestError as Error).name === "AbortError") {
          return;
        }
        setError(requestError instanceof Error ? requestError.message : "logs unavailable");
        failures.current += 1;
      } finally {
        if (inFlight.current === controller) {
          inFlight.current = null;
        }
        if (!stopped) {
          setLoading(false);
        }
      }
    }

    async function tick() {
      if (stopped) {
        return;
      }
      if (typeof document !== "undefined" && document.hidden) {
        timer = setTimeout(tick, pollIntervalMs);
        return;
      }
      await load();
      if (stopped) {
        return;
      }
      const delay =
        failures.current > 0
          ? backoffMs[Math.min(failures.current - 1, backoffMs.length - 1)]
          : pollIntervalMs;
      timer = setTimeout(tick, delay);
    }

    function onVisibilityChange() {
      if (typeof document !== "undefined" && !document.hidden) {
        if (timer) {
          clearTimeout(timer);
        }
        void tick();
      }
    }

    void tick();
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      stopped = true;
      if (timer) {
        clearTimeout(timer);
      }
      document.removeEventListener("visibilitychange", onVisibilityChange);
      inFlight.current?.abort();
      inFlight.current = null;
    };
  }, [filter.service, filter.level, refreshTick]);

  return {
    entries,
    loading,
    error,
    stale: entries.length > 0 && error !== null,
    lastUpdated,
    hasMore,
    refresh,
  };
}
