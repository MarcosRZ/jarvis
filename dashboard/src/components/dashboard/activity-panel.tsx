"use client";

import { Activity, CircleDot } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useLiveData } from "@/components/dashboard/live-data-provider";
import { formatRelativeTime } from "@/lib/live-data";

export function ActivityPanel() {
  const { snapshot, loading, error, sourceError, refresh } = useLiveData();
  const activity = snapshot?.activity ?? [];
  const issue = error ?? sourceError;

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b p-5">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Activity className="size-4 text-primary" />
            <CardTitle className="text-sm">Actividad reciente</CardTitle>
          </div>
          {issue && (
            <button
              type="button"
              className="text-xs font-medium text-primary underline-offset-4 hover:underline"
              onClick={refresh}
            >
              Retry
            </button>
          )}
        </div>
        <p className="text-xs text-muted-foreground">
          Docker events from this compose project
        </p>
      </CardHeader>
      <CardContent className="p-0">
        {loading ? (
          <div className="space-y-4 p-5">
            {Array.from({ length: 3 }, (_, index) => (
              <div key={index} className="space-y-2">
                <Skeleton className="h-4 w-40" />
                <Skeleton className="h-3 w-24" />
              </div>
            ))}
          </div>
        ) : activity.length === 0 ? (
          <p className="p-5 text-sm text-muted-foreground" role="status">
            {issue ? "Activity unavailable." : "No recent activity."}
          </p>
        ) : (
          <ol className="relative ml-6 border-l py-4">
            {activity.map((item) => (
              <li key={item.id} className="relative pb-4 pl-5 last:pb-0">
                <CircleDot className="absolute -left-[9px] top-0.5 size-4 bg-card text-primary" />
                <p className="text-sm font-medium leading-tight">{item.title}</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {item.description}
                </p>
                <p className="mt-0.5 text-[10px] uppercase tracking-wide text-muted-foreground/70">
                  {formatRelativeTime(item.time)}
                </p>
              </li>
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  );
}
