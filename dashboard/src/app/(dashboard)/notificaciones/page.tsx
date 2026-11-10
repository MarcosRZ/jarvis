import { PageHeader } from "@/components/dashboard/page-header";
import { NotificationsPanel } from "@/components/dashboard/notifications-panel";

export default function NotificationsPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Notificaciones"
        description="Alertas y eventos del sistema. Pulsa una notificacion para marcarla como leida."
      />

      <NotificationsPanel />
    </div>
  );
}
