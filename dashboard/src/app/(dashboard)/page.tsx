import Link from "next/link";
import { ArrowUpRight, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/page-header";
import { StatCards } from "@/components/dashboard/stat-cards";
import { ResourcesPanel } from "@/components/dashboard/resources-panel";
import { ActivityPanel } from "@/components/dashboard/activity-panel";

export default function GeneralPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="General"
        description="Estado de los servicios de M4nhattan: memoria, actividad y salud del sistema en un solo lugar."
        badge="operativo"
      >
        <Button variant="outline" size="sm">
          <RefreshCw className="size-4" />
          Actualizar
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
