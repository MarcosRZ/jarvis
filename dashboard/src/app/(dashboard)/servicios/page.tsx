import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/dashboard/page-header";
import { ServiceCard } from "@/components/dashboard/service-card";
import { services } from "@/lib/mock-data";

export default function ServicesPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Servicios"
        description={`${services.length} contenedores registrados. Estado, memoria, CPU y puertos de cada servicio.`}
      >
        <Button size="sm" disabled title="Disponible en una proxima version">
          <Plus className="size-4" />
          Nuevo servicio
        </Button>
      </PageHeader>

      <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
        {services.map((service) => (
          <ServiceCard key={service.id} service={service} />
        ))}
      </div>
    </div>
  );
}
