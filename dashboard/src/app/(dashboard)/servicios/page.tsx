"use client";

import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/dashboard/page-header";
import { ServiceCard } from "@/components/dashboard/service-card";
import { useLiveData } from "@/components/dashboard/live-data-provider";

function ServiceCardSkeleton() {
  return (
    <Card className="gap-0 py-0">
      <div className="flex items-start gap-3 border-b p-4">
        <Skeleton className="size-10 shrink-0" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-3 w-40" />
          <Skeleton className="h-2.5 w-32" />
        </div>
      </div>
      <div className="grid grid-cols-3 divide-x border-b">
        {Array.from({ length: 3 }, (_, index) => (
          <div key={index} className="space-y-2 px-2 py-3">
            <Skeleton className="mx-auto h-2.5 w-10" />
            <Skeleton className="mx-auto h-3.5 w-12" />
          </div>
        ))}
      </div>
      <div className="px-4 py-3">
        <Skeleton className="h-3 w-16" />
      </div>
    </Card>
  );
}

export default function ServicesPage() {
  const { snapshot, loading, error, sourceError, stale, refresh, refreshing } =
    useLiveData();
  const services = snapshot?.services ?? [];
  const issue = error ?? sourceError;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Servicios"
        description={
          loading
            ? "Loading registered containers..."
            : `${services.length} servicios registrados. Estado, memoria, CPU y puertos de cada servicio.`
        }
      >
        <Button
          variant="outline"
          size="sm"
          onClick={refresh}
          disabled={refreshing}
          aria-label="Retry live data"
        >
          {issue ? "Retry" : "Refresh"}
        </Button>
        <Button size="sm" disabled title="Disponible en una proxima version">
          <Plus className="size-4" />
          Nuevo servicio
        </Button>
      </PageHeader>

      {(issue || stale) && (
        <div
          className="flex items-center justify-between gap-2 border border-destructive/30 bg-destructive/5 px-4 py-2 text-xs text-destructive"
          role="status"
          aria-live="polite"
        >
          <span>
            {stale ? "Showing stale service data." : `Live data unavailable: ${issue}`}
          </span>
          <button
            type="button"
            className="font-medium underline-offset-4 hover:underline"
            onClick={refresh}
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {loading
          ? Array.from({ length: 6 }, (_, index) => <ServiceCardSkeleton key={index} />)
          : services.map((service) => <ServiceCard key={service.id} service={service} />)}
      </div>

      {!loading && services.length === 0 && (
        <p className="text-sm text-muted-foreground" role="status">
          No services found.
        </p>
      )}
    </div>
  );
}
