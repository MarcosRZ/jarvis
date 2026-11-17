import { Suspense } from "react";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { LearningForm } from "@/components/dashboard/learning-form";

function FormFallback() {
  return (
    <div className="space-y-6" aria-busy="true">
      <div className="space-y-2">
        <Skeleton className="h-8 w-56" />
        <Skeleton className="h-4 w-full max-w-xl" />
      </div>
      <Card className="gap-0 py-0">
        <div className="space-y-4 p-6">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-2/3" />
          <Skeleton className="h-24 w-full" />
        </div>
      </Card>
    </div>
  );
}

export default function LearningEditPage() {
  return (
    <Suspense fallback={<FormFallback />}>
      <LearningForm />
    </Suspense>
  );
}
