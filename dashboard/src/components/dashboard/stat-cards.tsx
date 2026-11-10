import {
  ArrowDownRight,
  ArrowUpRight,
  Clock,
  Cpu,
  MemoryStick,
  Minus,
  Server,
} from "lucide-react";
import { Card } from "@/components/ui/card";
import { stats } from "@/lib/mock-data";

const icons = {
  services: Server,
  memory: MemoryStick,
  cpu: Cpu,
  uptime: Clock,
};

const trendIcons = {
  up: ArrowUpRight,
  down: ArrowDownRight,
  flat: Minus,
};

export function StatCards() {
  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      {stats.map((stat) => {
        const Icon = icons[stat.icon];
        const TrendIcon = trendIcons[stat.trend];
        return (
          <Card key={stat.label} className="gap-0 py-0">
            <div className="flex items-start justify-between p-5">
              <div className="space-y-1">
                <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {stat.label}
                </p>
                <p className="text-2xl font-semibold tracking-tight">
                  {stat.value}
                </p>
                <p className="text-xs text-muted-foreground">{stat.detail}</p>
              </div>
              <div className="flex size-9 items-center justify-center border bg-muted/40">
                <Icon className="size-4 text-primary" />
              </div>
            </div>
            <div className="flex items-center gap-1.5 border-t px-5 py-2.5 text-xs text-muted-foreground">
              <TrendIcon className="size-3.5" />
              <span>{stat.trendLabel}</span>
            </div>
          </Card>
        );
      })}
    </div>
  );
}
