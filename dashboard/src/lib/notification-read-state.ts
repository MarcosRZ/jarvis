"use client";

// Read state is local to this browser. Persisting it on the collector later
// only requires replacing this module's storage calls; consumers stay the same.
import { useSyncExternalStore } from "react";

const storageKey = "m4nhattan-read-notifications:v1";
const empty: string[] = [];
const listeners = new Set<() => void>();
let cache: string[] | null = null;

function readFromStorage(): string[] {
  if (typeof window === "undefined") {
    return empty;
  }
  if (cache !== null) {
    return cache;
  }
  try {
    const raw = window.localStorage.getItem(storageKey);
    if (!raw) {
      cache = empty;
      return cache;
    }
    const parsed = JSON.parse(raw) as unknown;
    cache = Array.isArray(parsed)
      ? parsed.filter((value): value is string => typeof value === "string")
      : empty;
  } catch {
    cache = empty;
  }
  return cache;
}

function writeToStorage(ids: string[]) {
  cache = ids;
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(storageKey, JSON.stringify(ids));
    } catch {
      return;
    }
  }
  for (const listener of listeners) {
    listener();
  }
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  const onStorage = (event: StorageEvent) => {
    if (event.key === storageKey) {
      cache = null;
      listener();
    }
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", onStorage);
  };
}

function getSnapshot(): string[] {
  return readFromStorage();
}

function getServerSnapshot(): string[] {
  return empty;
}

export function markNotificationRead(id: string) {
  const current = readFromStorage();
  if (current.includes(id)) {
    return;
  }
  writeToStorage([...current, id]);
}

export function markNotificationsRead(ids: string[]) {
  const current = readFromStorage();
  const next = Array.from(new Set([...current, ...ids]));
  if (next.length === current.length) {
    return;
  }
  writeToStorage(next);
}

export function useReadNotificationIds(): string[] {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function readNotificationIds(): string[] {
  return readFromStorage();
}
