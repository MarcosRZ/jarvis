import { MemoryStick } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { services } from "@/lib/mock-data";

const palette = [
  "bg-chart-1",
  "bg-chart-2",
  "bg-chart-3",
  "bg-chart-4",
  "bg-chart-5",
  "bg-muted-foreground",
];

export function ResourcesPanel() {
  const active = services.filter((service) => service.memoryLimitMb > 0);
  const used = active.reduce((total, service) => total + service.memoryMb, 0);
  const limit = active.reduce(
    (total, service) => total + service.memoryLimitMb,
    0,
  );
  const usedPct = Math.round((used / limit) * 100);

  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <MemoryStick className="size-4 text-primary" />
            <CardTitle className="text-sm">Recursos</CardTitle>
          </div>
          <span className="text-xs text-muted-foreground">host</span>
        </div>
        <p className="text-xs text-muted-foreground">
          Memoria reservada por contenedor
        </p>
      </CardHeader>
      <CardContent className="space-y-4 p-5">
        <div>
          <div className="flex items-end justify-between">
            <p className="text-2xl font-semibold tracking-tight tabular-nums">
              {(used / 1024).toFixed(1)} GB
            </p>
            <p className="text-xs text-muted-foreground tabular-nums">
              de {(limit / 1024).toFixed(1)} GB
            </p>
          </div>
          <div className="mt-2 flex h-2 w-full overflow-hidden bg-muted">
            {active.map((service, index) => (
              <div
                key={service.id}
                className={palette[index % palette.length]}
                style={{
                  width: `${(service.memoryMb / limit) * 100}%`,
                }}
                title={`${service.name}: ${service.memoryMb} MB`}
              />
            ))}
          </div>
          <p className="mt-1.5 text-[11px] text-muted-foreground">
            {usedPct}% del limite agregado en uso
          </p>
        </div>

        <div className="space-y-2.5">
          {active
            .slice()
            .sort((a, b) => b.memoryMb - a.memoryMb)
            .map((service) => {
              const index = active.findIndex((item) => item.id === service.id);
              const pct = Math.round((service.memoryMb / limit) * 100);
              return (
                <div key={service.id} className="space-y-1">
                  <div className="flex items-center justify-between text-xs">
                    <span className="flex items-center gap-2">
                      <span
                        className={`size-2 ${palette[index % palette.length]}`}
                      />
                      {service.name}
                    </span>
                    <span className="font-mono text-[10px] tabular-nums text-muted-foreground">
                      {service.memoryMb} MB
                    </span>
                  </div>
                  <div className="h-1 w-full bg-muted">
                    <div
                      className={palette[index % palette.length]}
                      style={{ width: `${pct}%`, height: "100%" }}
                    />
                  </div>
                </div>
              );
            })}
        </div>
      </CardContent>
    </Card>
  );
}
