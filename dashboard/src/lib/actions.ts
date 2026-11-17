import type { SourceStatus } from "@/lib/live-data";

export type CatalogAction = {
  id: string;
  name: string;
  description: string;
  entity_id: string;
  domain: string;
  service: string;
  data?: Record<string, unknown>;
  aliases?: string[];
};

export type ActionsResponse = {
  version: number;
  generatedAt: string;
  source: SourceStatus;
  entityCount: number;
  actions: CatalogAction[];
};
