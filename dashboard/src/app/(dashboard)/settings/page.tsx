import { PageHeader } from "@/components/dashboard/page-header";
import { PromptSettings } from "@/components/dashboard/prompt-settings";

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        description="Configure the language model prompt used for questions and conversation. Device actions keep their own safety rules."
      />
      <PromptSettings />
    </div>
  );
}
