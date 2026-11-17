"use client";

import { MemoryStick } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useLiveData } from "@/components/dashboard/live-data-provider";
import { formatMemory } from "@/lib/live-data";

const palette = [
  "bg-chart-1",
  "bg-chart-2",
  "bg-chart-3",
  "bg-chart-4",
  "bg-chart-5",
  "bg-muted-foreground",
];

export function ResourcesPanel() {
  const { snapshot, loading, error, sourceError } = useLiveData();
  const issue = error ?? sourceError;
  const active = (snapshot?.services ?? []).filter(
    (service) => service.memoryMb !== null,
  );
  const used = active.reduce((total, service) => total + (service.memoryMb ?? 0), 0);
  const limit = snapshot?.metrics.memory.limitMb ?? null;
  const knownLimit = limit !== null && limit > 0;
  const usedPct = knownLimit ? Math.round((used / limit) * 100) : null;

  return (
    <Card className="gap-0 overflow-hidden py-0">
      <CardHeader className="border-b p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <MemoryStick className="size-4 text-primary" />
            <CardTitle className="text-sm">Recursos</CardTitle>
          </div>
          <span className="text-xs text-muted-foreground">containers</span>
        </div>
        <p className="text-xs text-muted-foreground">
          Memoria en uso por contenedor
        </p>
      </CardHeader>
      <CardContent className="space-y-4 p-5">
        {loading ? (
          <div className="space-y-3">
            <Skeleton className="h-8 w-28" />
            <Skeleton className="h-2 w-full" />
            <Skeleton className="h-3 w-40" />
          </div>
        ) : active.length === 0 ? (
          <p className="text-sm text-muted-foreground" role="status">
            No memory data
          </p>
        ) : (
          <>
            <div>
              <div className="flex items-end justify-between">
                <p className="text-2xl font-semibold tracking-tight tabular-nums">
                  {formatMemory(used)}
                </p>
                <p className="text-xs text-muted-foreground tabular-nums">
                  {knownLimit ? `of ${formatMemory(limit)}` : "no container limits set"}
                </p>
              </div>
              <div className="mt-2 flex h-2 w-full overflow-hidden bg-muted">
                {active.map((service, index) => (
                  <div
                    key={service.id}
                    className={palette[index % palette.length]}
                    style={{
                      width: `${used > 0 ? ((service.memoryMb ?? 0) / used) * 100 : 0}%`,
                    }}
                    title={`${service.name}: ${formatMemory(service.memoryMb)}`}
                  />
                ))}
              </div>
              <p className="mt-1.5 text-[11px] text-muted-foreground">
                {usedPct === null ? "Container memory usage" : `${usedPct}% of the configured limit`}
              </p>
            </div>

            <div className="space-y-2.5">
              {active
                .slice()
                .sort((a, b) => (b.memoryMb ?? 0) - (a.memoryMb ?? 0))
                .map((service) => {
                  const index = active.findIndex((item) => item.id === service.id);
                  const pct = used > 0 ? Math.round(((service.memoryMb ?? 0) / used) * 100) : 0;
                  return (
                    <div key={service.id} className="space-y-1">
                      <div className="flex items-center justify-between gap-2 text-xs">
                        <span className="flex min-w-0 items-center gap-2">
                          <span
                            className={`size-2 shrink-0 ${palette[index % palette.length]}`}
                          />
                          <span className="truncate">{service.name}</span>
                        </span>
                        <span className="shrink-0 font-mono text-[10px] tabular-nums text-muted-foreground">
                          {formatMemory(service.memoryMb)}
                        </span>
                      </div>
                      <div className="h-1 w-full bg-muted">
                        <div
                          className={palette[index % palette.length]}
                          style={{ width: `${pct}%`, height: "100%" }}
                        />
                      </div>
                    </div>
                  );
                })}
            </div>
          </>
        )}
        {issue && (
          <p className="text-xs text-destructive" role="status">
            {sourceError ?? (snapshot ? "Showing stale data." : `Live data unavailable: ${error}`)}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
