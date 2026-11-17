"use client";

import Link from "next/link";
import { ArrowUpRight, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/page-header";
import { StatCards } from "@/components/dashboard/stat-cards";
import { ResourcesPanel } from "@/components/dashboard/resources-panel";
import { ActivityPanel } from "@/components/dashboard/activity-panel";
import { useLiveData } from "@/components/dashboard/live-data-provider";

export default function GeneralPage() {
  const { snapshot, loading, error, sourceError, refresh, refreshing } = useLiveData();
  const services = snapshot?.metrics.services;
  const badge = error || sourceError
    ? "unavailable"
    : loading || !services
      ? "loading"
      : services.stopped > 0
        ? "degraded"
        : "operational";

  return (
    <div className="space-y-6">
      <PageHeader
        title="General"
        description="Estado de los servicios de M4nhattan: memoria, actividad y salud del sistema en un solo lugar."
        badge={badge}
      >
        <Button variant="outline" size="sm" onClick={refresh} disabled={refreshing}>
          <RefreshCw className={`size-4 ${refreshing ? "animate-spin" : ""}`} />
          {error || sourceError ? "Retry" : "Actualizar"}
        </Button>
        <Button size="sm" asChild>
          <Link href="/servicios">
            Ver servicios
            <ArrowUpRight className="size-4" />
          </Link>
        </Button>
      </PageHeader>

      <StatCards />

      <div className="grid gap-4 xl:grid-cols-2">
        <ResourcesPanel />
        <ActivityPanel />
      </div>
    </div>
  );
}
