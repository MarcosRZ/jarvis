import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/page-header";
import { LogsPanel } from "@/components/dashboard/logs-panel";

export default function LogsPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Registros"
        description="Ultimas entradas de todos los contenedores. Filtra por nivel o por servicio."
      >
        <Button variant="outline" size="sm" disabled title="Disponible en una proxima version">
          <Download className="size-4" />
          Exportar
        </Button>
      </PageHeader>

      <LogsPanel heightClassName="h-[calc(100vh-18rem)] min-h-[360px]" />
    </div>
  );
}
