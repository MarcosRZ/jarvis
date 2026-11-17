"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { LearningResponse } from "@/lib/learning";

type LearningState = {
  data: LearningResponse | null;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  stale: boolean;
  refresh: () => void;
};

const pollIntervalMs = 30000;
const pendingPollIntervalMs = 5000;
const backoffMs = [1000, 2000, 4000, 8000, 16000, 30000];

export function useLearning(): LearningState {
  const [data, setData] = useState<LearningResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshTick, setRefreshTick] = useState(0);
  const inFlight = useRef<AbortController | null>(null);
  const failures = useRef(0);
  const manual = useRef(false);
  const pending = useRef(0);

  const refresh = useCallback(() => {
    failures.current = 0;
    manual.current = true;
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
      setRefreshing(manual.current);
      try {
        const response = await fetch("/api/learning", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }
        const next = (await response.json()) as LearningResponse;
        if (stopped) {
          return;
        }
        pending.current = next.pendingOperations ?? 0;
        setData(next);
        setError(null);
        failures.current = 0;
      } catch (requestError) {
        if (stopped || (requestError as Error).name === "AbortError") {
          return;
        }
        setError(
          requestError instanceof Error
            ? requestError.message
            : "learning snapshot unavailable",
        );
        failures.current += 1;
      } finally {
        if (inFlight.current === controller) {
          inFlight.current = null;
        }
        if (!stopped) {
          setRefreshing(false);
        }
        manual.current = false;
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
          : pending.current > 0
            ? pendingPollIntervalMs
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

  return {
    data,
    loading: data === null && error === null,
    refreshing,
    error,
    stale: data !== null && error !== null,
    refresh,
  };
}
