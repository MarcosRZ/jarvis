"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import type { Snapshot } from "@/lib/live-data";

type LiveDataState = {
  snapshot: Snapshot | null;
  loading: boolean;
  error: string | null;
  sourceError: string | null;
  stale: boolean;
  lastUpdated: string | null;
  refreshing: boolean;
  refresh: () => void;
};

const LiveDataContext = createContext<LiveDataState | null>(null);
const pollIntervalMs = 5000;
const backoffMs = [1000, 2000, 4000, 8000, 16000, 30000];

export function LiveDataProvider({ children }: { children: React.ReactNode }) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshTick, setRefreshTick] = useState(0);
  const inFlight = useRef<AbortController | null>(null);
  const failures = useRef(0);

  const refresh = useCallback(() => {
    failures.current = 0;
    setRefreshTick((tick) => tick + 1);
  }, []);

  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    async function load() {
      if (stopped) {
        return;
      }
      const controller = new AbortController();
      inFlight.current?.abort();
      inFlight.current = controller;
      setRefreshing(true);
      try {
        const response = await fetch("/api/status", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const data = (await response.json()) as Snapshot;
        if (stopped) {
          return;
        }
        setSnapshot(data);
        setLastUpdated(data.generatedAt ?? new Date().toISOString());
        setError(null);
        failures.current = 0;
      } catch (requestError) {
        if (stopped || (requestError as Error).name === "AbortError") {
          return;
        }
        setError(requestError instanceof Error ? requestError.message : "live data unavailable");
        failures.current += 1;
      } finally {
        if (inFlight.current === controller) {
          inFlight.current = null;
        }
        if (!stopped) {
          setRefreshing(false);
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
  }, [refreshTick]);

  const sourceError =
    snapshot?.sources.docker.state === "unavailable"
      ? (snapshot.sources.docker.message ?? "Docker API is not available.")
      : null;
  const value: LiveDataState = {
    snapshot,
    loading: snapshot === null && error === null,
    error,
    sourceError,
    stale: snapshot !== null && error !== null,
    lastUpdated,
    refreshing,
    refresh,
  };

  return <LiveDataContext.Provider value={value}>{children}</LiveDataContext.Provider>;
}

export function useLiveData(): LiveDataState {
  const context = useContext(LiveDataContext);
  if (!context) {
    throw new Error("useLiveData must be used inside LiveDataProvider");
  }
  return context;
}
