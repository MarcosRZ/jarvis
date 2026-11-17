import { useState } from "react";
import {
  AudioLines,
  Box,
  Brain,
  Check,
  House,
  LayoutDashboard,
  Loader2,
  Mic,
  RotateCw,
  Waves,
  X,
} from "lucide-react";
import { cn } from "cn";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useLiveData } from "@/components/dashboard/live-data-provider";
import {
  formatDuration,
  formatMemory,
  formatPercent,
  unknownValue,
  type Service,
  type ServiceStatus,
} from "@/lib/live-data";

const icons = {
  model: Waves,
  voice: AudioLines,
  mic: Mic,
  brain: Brain,
  home: House,
  dashboard: LayoutDashboard,
  unknown: Box,
};

const statusStyles: Record<
  ServiceStatus,
  { label: string; dot: string; text: string; border: string }
> = {
  online: {
    label: "Online",
    dot: "bg-emerald-500",
    text: "text-emerald-600 dark:text-emerald-400",
    border: "border-emerald-500/30",
  },
  degraded: {
    label: "Degraded",
    dot: "bg-amber-500",
    text: "text-amber-600 dark:text-amber-400",
    border: "border-amber-500/30",
  },
  offline: {
    label: "Offline",
    dot: "bg-destructive",
    text: "text-destructive",
    border: "border-destructive/30",
  },
  restarting: {
    label: "Restarting",
    dot: "bg-primary",
    text: "text-primary",
    border: "border-primary/30",
  },
};

function restartErrorMessage(status: number): string {
  if (status === 403) {
    return "Este servicio no se puede reiniciar desde el panel.";
  }
  if (status === 404) {
    return "No hay un contenedor para este servicio.";
  }
  if (status === 504) {
    return "El reinicio tardó demasiado. Comprueba el estado del servicio.";
  }
  return "No se pudo reiniciar el servicio.";
}

export function ServiceCard({ service }: { service: Service }) {
  const { refresh } = useLiveData();
  const Icon = icons[service.icon] ?? icons.unknown;
  const status = statusStyles[service.status];
  const canRestart = service.image !== "" && service.id !== "dashboard";
  const [confirming, setConfirming] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function restart() {
    setPending(true);
    setError(null);
    try {
      const response = await fetch("/api/services/restart", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id: service.id }),
      });
      if (!response.ok) {
        setError(restartErrorMessage(response.status));
        return;
      }
      setConfirming(false);
      refresh();
    } catch {
      setError("No se pudo contactar con el colector.");
    } finally {
      setPending(false);
    }
  }

  return (
    <Card className="gap-0 overflow-hidden py-0">
      <div className="flex items-start gap-3 border-b p-4">
        <div className="flex size-10 shrink-0 items-center justify-center border bg-muted/40">
          <Icon className="size-5 text-primary" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-semibold">{service.name}</h3>
            {service.id === "dashboard" && (
              <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                self
              </span>
            )}
          </div>
          <p className="truncate text-xs text-muted-foreground">
            {service.description}
          </p>
          <p className="mt-1 truncate font-mono text-[10px] text-muted-foreground/80">
            {service.image || "no container"}
          </p>
        </div>
        <Badge
          variant="outline"
          className={cn(
            "gap-1.5 rounded-none px-2 py-0.5 text-[10px]",
            status.border,
            status.text,
          )}
        >
          <span className={cn("size-1.5", status.dot)} />
          {status.label}
        </Badge>
      </div>

      <div className="grid grid-cols-3 divide-x border-b text-center">
        <div className="px-2 py-2.5">
          <p className="text-[10px] uppercase tracking-wide text-muted-foreground">
            CPU
          </p>
          <p className="text-sm font-medium tabular-nums">
            {formatPercent(service.cpu)}
          </p>
        </div>
        <div className="px-2 py-2.5">
          <p className="text-[10px] uppercase tracking-wide text-muted-foreground">
            Memoria
          </p>
          <p className="text-sm font-medium tabular-nums">
            {formatMemory(service.memoryMb)}
          </p>
        </div>
        <div className="px-2 py-2.5">
          <p className="text-[10px] uppercase tracking-wide text-muted-foreground">
            Uptime
          </p>
          <p className="text-sm font-medium tabular-nums">
            {formatDuration(service.uptimeSeconds)}
          </p>
        </div>
      </div>

      <div className="mt-auto flex flex-wrap items-center justify-between gap-2 px-4 py-3">
        <Badge
          variant="outline"
          title="Puerto publicado"
          className="rounded-none border-primary/30 px-2 py-0 text-[10px] font-mono text-primary"
        >
          {service.port === null ? unknownValue : `:${service.port}`}
        </Badge>
        {canRestart &&
          (confirming ? (
            <div className="flex items-center gap-1.5">
              <span className="text-[10px] text-muted-foreground">
                ¿Reiniciar?
              </span>
              <Button
                variant="destructive"
                size="icon-xs"
                onClick={restart}
                disabled={pending}
                aria-label={`Confirm restart of ${service.name}`}
              >
                {pending ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Check className="size-3.5" />
                )}
              </Button>
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setConfirming(false)}
                disabled={pending}
                aria-label={`Cancel restart of ${service.name}`}
              >
                <X className="size-3.5" />
              </Button>
            </div>
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={() => {
                setError(null);
                setConfirming(true);
              }}
              aria-label={`Restart ${service.name}`}
            >
              <RotateCw className="size-3.5" />
              Reiniciar
            </Button>
          ))}
      </div>
      {error && (
        <p className="px-4 pb-3 text-[10px] text-destructive" role="alert">
          {error}
        </p>
      )}
    </Card>
  );
}
