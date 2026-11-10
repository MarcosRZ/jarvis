"use client";

import { useState } from "react";
import {
  Bell,
  CheckCheck,
  CircleCheck,
  CircleX,
  Info,
  TriangleExclamationPoint,
} from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { notifications as seed } from "@/lib/mock-data";

const icons = {
  error: CircleX,
  warning: TriangleExclamationPoint,
  success: CircleCheck,
  info: Info,
};

const styles = {
  error: "text-destructive",
  warning: "text-amber-600 dark:text-amber-400",
  success: "text-emerald-600 dark:text-emerald-400",
  info: "text-primary",
};

export function NotificationsPanel() {
  const [read, setRead] = useState<number[]>(
    seed.filter((notification) => notification.read).map((item) => item.id),
  );
  const unread = seed.length - read.length;

  return (
    <Card className="gap-0 py-0" id="notifications">
      <CardHeader className="border-b p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Bell className="size-4 text-primary" />
            <CardTitle className="text-sm">Notificaciones</CardTitle>
          </div>
          <span className="text-xs text-muted-foreground">
            {unread} sin leer
          </span>
        </div>
        <p className="text-xs text-muted-foreground">
          Alertas y eventos del sistema
        </p>
      </CardHeader>
      <CardContent className="p-0">
        <ul className="divide-y">
          {seed.map((notification) => {
            const Icon = icons[notification.type];
            const isRead = read.includes(notification.id);
            return (
              <li key={notification.id}>
                <button
                  type="button"
                  onClick={() =>
                    setRead((current) =>
                      current.includes(notification.id)
                        ? current
                        : [...current, notification.id],
                    )
                  }
                  className={cn(
                    "flex w-full items-start gap-3 p-4 text-left transition-colors hover:bg-muted/50",
                    isRead && "opacity-60",
                  )}
                >
                  <Icon
                    className={cn("mt-0.5 size-4 shrink-0", styles[notification.type])}
                  />
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">
                        {notification.title}
                      </span>
                      {!isRead && (
                        <span className="size-1.5 shrink-0 bg-primary" />
                      )}
                    </span>
                    <span className="text-xs leading-relaxed text-muted-foreground">
                      {notification.description}
                    </span>
                    <span className="text-[10px] uppercase tracking-wide text-muted-foreground/70">
                      {notification.time}
                    </span>
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
        <div className="border-t p-3">
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-center text-xs"
            onClick={() => setRead(seed.map((notification) => notification.id))}
            disabled={unread === 0}
          >
            <CheckCheck className="size-3.5" />
            Marcar todas como leidas
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
