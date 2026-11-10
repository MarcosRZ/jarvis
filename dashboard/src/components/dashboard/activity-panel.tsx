import { Activity, CircleDot } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { activity } from "@/lib/mock-data";

export function ActivityPanel() {
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b p-5">
        <div className="flex items-center gap-2">
          <Activity className="size-4 text-primary" />
          <CardTitle className="text-sm">Actividad reciente</CardTitle>
        </div>
        <p className="text-xs text-muted-foreground">
          Cambios en la plataforma
        </p>
      </CardHeader>
      <CardContent className="p-0">
        <ol className="relative ml-6 border-l py-4">
          {activity.map((item) => (
            <li key={item.id} className="relative pb-4 pl-5 last:pb-0">
              <CircleDot className="absolute -left-[9px] top-0.5 size-4 bg-card text-primary" />
              <p className="text-sm font-medium leading-tight">{item.title}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {item.description}
              </p>
              <p className="mt-0.5 text-[10px] uppercase tracking-wide text-muted-foreground/70">
                {item.time}
              </p>
            </li>
          ))}
        </ol>
      </CardContent>
    </Card>
  );
}
