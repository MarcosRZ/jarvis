import type { SourceStatus } from "@/lib/live-data";

export type LearningEntry = {
  text: string;
  device?: string;
  name?: string;
  kind?: string;
  action?: string;
  score: number;
  successes: number;
  failures: number;
  updated: string;
};

export type LearningResponse = {
  version: number;
  generatedAt: string | null;
  receivedAt: string | null;
  source: SourceStatus;
  entries: LearningEntry[];
};

export type LearningStatus = "active" | "penalized" | "inactive";

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
  return entry.action ?? entry.device ?? "unknown target";
}
