import { PageHeader } from "@/components/dashboard/page-header";
import { SettingsPanel } from "@/components/dashboard/settings-panel";

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        description="General, per-service and voice client overrides. Empty fields keep using the .env files."
      />
      <SettingsPanel />
    </div>
  );
}
