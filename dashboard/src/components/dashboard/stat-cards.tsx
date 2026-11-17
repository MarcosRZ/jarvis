"use client";

import {
  ArrowDownRight,
  ArrowUpRight,
  Clock,
  Cpu,
  MemoryStick,
  Minus,
  Server,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useLiveData } from "@/components/dashboard/live-data-provider";
import { deriveStats } from "@/lib/live-data";

const icons = {
  services: Server,
  memory: MemoryStick,
  cpu: Cpu,
  uptime: Clock,
};

const trendIcons = {
  up: ArrowUpRight,
  down: ArrowDownRight,
  flat: Minus,
};

export function StatCards() {
  const { snapshot, loading, error, sourceError, stale, lastUpdated, refresh, refreshing } =
    useLiveData();
  const stats = deriveStats(snapshot);
  const issue = error ?? sourceError;
  const state = sourceError
    ? "Docker source unavailable"
    : error
      ? stale
        ? "Stale data"
        : "Live data unavailable"
      : lastUpdated
        ? "Live"
        : "";

  return (
    <div className="space-y-2">
      <div className="flex min-h-5 items-center justify-between gap-2 text-xs text-muted-foreground" role="status" aria-live="polite">
        <span>{state}</span>
        {issue && (
          <Button variant="outline" size="sm" className="h-7 px-2 text-xs" onClick={refresh} disabled={refreshing} aria-label="Retry live data">
            Retry
          </Button>
        )}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {loading
          ? Array.from({ length: 4 }, (_, index) => (
              <Card key={index} className="gap-0 py-0">
                <div className="space-y-3 p-5">
                  <Skeleton className="h-3 w-28" />
                  <Skeleton className="h-7 w-20" />
                  <Skeleton className="h-3 w-32" />
                </div>
                <div className="border-t px-5 py-2.5">
                  <Skeleton className="h-3 w-24" />
                </div>
              </Card>
            ))
          : stats.map((stat) => {
              const Icon = icons[stat.icon];
              const TrendIcon = trendIcons[stat.trend];
              return (
                <Card key={stat.label} className="gap-0 py-0">
                  <div className="flex items-start justify-between p-5">
                    <div className="space-y-1">
                      <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                        {stat.label}
                      </p>
                      <p className="text-2xl font-semibold tracking-tight">
                        {stat.value}
                      </p>
                      <p className="text-xs text-muted-foreground">{stat.detail}</p>
                    </div>
                    <div className="flex size-9 items-center justify-center border bg-muted/40">
                      <Icon className="size-4 text-primary" />
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 border-t px-5 py-2.5 text-xs text-muted-foreground">
                    <TrendIcon className="size-3.5" />
                    <span>{stat.trendLabel}</span>
                  </div>
                </Card>
              );
            })}
      </div>
    </div>
  );
}
