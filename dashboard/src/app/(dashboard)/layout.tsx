import { AppSidebar } from "@/components/dashboard/sidebar";
import { Topbar } from "@/components/dashboard/topbar";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="min-h-screen bg-background">
      <AppSidebar />
      <div className="flex min-h-screen flex-col lg:pl-64">
        <Topbar />
        <main className="flex-1 p-4 sm:p-6">{children}</main>
        <footer className="flex flex-col gap-1 border-t px-4 py-4 text-[11px] text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <span>M4nhattan dashboard v0.1.0 - vista previa con datos simulados.</span>
          <span>
            La integracion con Docker, reinicios y ajustes de memoria llegan en la
            proxima fase.
          </span>
        </footer>
      </div>
    </div>
  );
}
