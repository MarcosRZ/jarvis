"use client";

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
import { Skeleton } from "@/components/ui/skeleton";
import { useLiveData } from "@/components/dashboard/live-data-provider";
import { formatRelativeTime } from "@/lib/live-data";
import {
  markNotificationRead,
  markNotificationsRead,
  useReadNotificationIds,
} from "@/lib/notification-read-state";

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
  const { snapshot, loading, error, sourceError, refresh } = useLiveData();
  const readIds = useReadNotificationIds();
  const notifications = snapshot?.notifications ?? [];
  const issue = error ?? sourceError;
  const unread = notifications.filter((notification) => !readIds.includes(notification.id));

  return (
    <Card className="gap-0 py-0" id="notifications">
      <CardHeader className="border-b p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Bell className="size-4 text-primary" />
            <CardTitle className="text-sm">Notificaciones</CardTitle>
          </div>
          <span className="text-xs text-muted-foreground" role="status" aria-live="polite">
            {loading ? "Loading" : `${unread.length} unread`}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">
          Alertas derivadas del estado de los contenedores
        </p>
      </CardHeader>
      <CardContent className="p-0">
        {loading ? (
          <div className="space-y-4 p-4">
            {Array.from({ length: 3 }, (_, index) => (
              <div key={index} className="flex items-start gap-3">
                <Skeleton className="mt-0.5 size-4" />
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-4 w-40" />
                  <Skeleton className="h-3 w-56" />
                  <Skeleton className="h-2.5 w-20" />
                </div>
              </div>
            ))}
          </div>
        ) : notifications.length === 0 ? (
          <div className="flex items-center justify-between gap-2 p-5" role="status">
            <p className="text-sm text-muted-foreground">
              {issue ? "Notifications unavailable." : "No notifications."}
            </p>
            {issue && (
              <button
                type="button"
                className="text-xs font-medium text-primary underline-offset-4 hover:underline"
                onClick={refresh}
              >
                Retry
              </button>
            )}
          </div>
        ) : (
          <ul className="divide-y">
            {notifications.map((notification) => {
              const Icon = icons[notification.type];
              const isRead = readIds.includes(notification.id);
              return (
                <li key={notification.id}>
                  <button
                    type="button"
                    onClick={() => markNotificationRead(notification.id)}
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
                        {formatRelativeTime(notification.time)}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
        {notifications.length > 0 && (
          <div className="border-t p-3">
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-center text-xs"
              onClick={() => markNotificationsRead(notifications.map((notification) => notification.id))}
              disabled={unread.length === 0}
            >
              <CheckCheck className="size-3.5" />
              Marcar todas como leidas
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
