import type { SourceStatus } from "@/lib/live-data";

export type LearningEntry = {
  text: string;
  device?: string;
  name?: string;
  kind?: string;
  action?: string;
  template?: string;
  reply?: string;
  pending?: boolean;
  score: number;
  successes: number;
  failures: number;
  updated: string;
};

export type LearningEntryInput = Omit<LearningEntry, "updated"> & {
  updated?: string;
};

export type LearningOperationInput =
  | { kind: "upsert"; originalText?: string; entry: LearningEntryInput }
  | { kind: "delete"; texts: string[] };

export type LearningOperation = {
  id: number;
  kind: "upsert" | "delete";
  originalText?: string;
  entry?: LearningEntry;
  texts?: string[];
  createdAt: string;
};

export type LearningResponse = {
  version: number;
  generatedAt: string | null;
  receivedAt: string | null;
  source: SourceStatus;
  entries: LearningEntry[];
  pendingOperations: number;
  pendingSince: string | null;
};

export type LearningStatus = "active" | "penalized" | "inactive";

const plainLetters: Record<string, string> = {
  á: "a",
  é: "e",
  í: "i",
  ó: "o",
  ú: "u",
  ü: "u",
  ñ: "n",
};

const fillers = new Set(["por", "favor", "porfa", "please", "gracias"]);

// normalizeLearningText mirrors the client normalization: lowercase, no Spanish
// accents, punctuation or filler words. The client applies the same key before
// it stores an association.
export function normalizeLearningText(text: string): string {
  const lower = text
    .toLowerCase()
    .replace(/[áéíóúüñ]/g, (letter) => plainLetters[letter] ?? letter);
  return lower
    .split(/[^\p{L}\p{N}]+/u)
    .filter(Boolean)
    .filter((word) => !fillers.has(word))
    .join(" ");
}

export function learningStatus(entry: LearningEntry): LearningStatus {
  if (entry.score > 0) {
    return "active";
  }
  if (entry.score < 0) {
    return "penalized";
  }
  return "inactive";
}

export function targetID(entry: LearningEntry): string {
  if (entry.pending) {
    return "pending target";
  }
  if (entry.action) {
    return entry.action;
  }
  if (entry.device) {
    return entry.device;
  }
  if (entry.template) {
    return `template: ${entry.template}`;
  }
  if (entry.reply) {
    return `reply: ${entry.reply}`;
  }
  return "unknown target";
}

export async function queueLearningOperation(
  operation: LearningOperationInput,
): Promise<{ operation: LearningOperation; pendingOperations: number }> {
  const response = await fetch("/api/learning/operations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(operation),
  });
  const data = (await response.json()) as {
    operation?: LearningOperation;
    pendingOperations?: number;
    error?: string;
  };
  if (!response.ok || !data.operation) {
    throw new Error(data.error ?? `HTTP ${response.status}`);
  }
  return {
    operation: data.operation,
    pendingOperations: data.pendingOperations ?? 0,
  };
}
