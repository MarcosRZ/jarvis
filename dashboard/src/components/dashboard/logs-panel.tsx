"use client";

import { Radio } from "lucide-react";
import { cn } from "cn";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLiveData } from "@/components/dashboard/live-data-provider";
import { useLogs, type LogFilter } from "@/components/dashboard/use-logs";
import { formatClock } from "@/lib/live-data";

const levelStyles: Record<string, string> = {
  info: "border-primary/30 text-primary",
  warn: "border-amber-500/30 text-amber-600 dark:text-amber-400",
  error: "border-destructive/30 text-destructive",
  debug: "border-border text-muted-foreground",
};

type Filter = {
  value: string;
  label: string;
};

function filterFor(services: { id: string }[]): Filter[] {
  const filters: Filter[] = [
    { value: "all", label: "Todos" },
    { value: "errors", label: "Errores" },
  ];
  for (const service of services) {
    filters.push({ value: service.id, label: service.id });
  }
  return filters;
}

function logFilter(value: string): LogFilter {
  if (value === "errors") {
    return { level: "error" };
  }
  if (value === "all") {
    return {};
  }
  return { service: value };
}

function LogTable({
  value,
  heightClassName,
}: {
  value: string;
  heightClassName: string;
}) {
  const filter = logFilter(value);
  const { entries, loading, error, stale, refresh } = useLogs(filter);

  if (loading) {
    return (
      <div className="space-y-3 p-4">
        {Array.from({ length: 6 }, (_, index) => (
          <Skeleton key={index} className="h-5 w-full" />
        ))}
      </div>
    );
  }

  return (
    <div>
      {(error || stale) && (
        <div
          className="flex items-center justify-between gap-2 border-b px-4 py-2 text-xs text-destructive"
          role="status"
          aria-live="polite"
        >
          <span>{stale ? "Showing stale log entries." : `Logs unavailable: ${error}`}</span>
          <button
            type="button"
            className="font-medium underline-offset-4 hover:underline"
            onClick={refresh}
          >
            Retry
          </button>
        </div>
      )}
      <ScrollArea
        className={heightClassName}
        scrollbars={["vertical", "horizontal"]}
      >
        <Table containerClassName="overflow-visible">
          <TableHeader className="sticky top-0 bg-card">
            <TableRow>
              <TableHead className="w-20 font-mono text-[10px] uppercase">
                Hora
              </TableHead>
              <TableHead className="w-16 text-[10px] uppercase">Nivel</TableHead>
              <TableHead className="w-28 text-[10px] uppercase">
                Servicio
              </TableHead>
              <TableHead className="text-[10px] uppercase">Mensaje</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((log) => (
              <TableRow key={log.id} className="font-mono text-xs">
                <TableCell className="whitespace-nowrap py-2 text-muted-foreground">
                  {formatClock(log.time)}
                </TableCell>
                <TableCell className="py-2">
                  <Badge
                    variant="outline"
                    className={cn(
                      "rounded-none px-1.5 py-0 text-[10px] uppercase",
                      levelStyles[log.level],
                    )}
                  >
                    {log.level}
                  </Badge>
                </TableCell>
                <TableCell className="whitespace-nowrap py-2 text-muted-foreground">
                  {log.service}
                </TableCell>
                <TableCell className="py-2">{log.message}</TableCell>
              </TableRow>
            ))}
            {entries.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={4}
                  className="py-10 text-center text-xs text-muted-foreground"
                >
                  No log entries for this filter.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </ScrollArea>
    </div>
  );
}

export function LogsPanel({
  heightClassName = "h-[320px]",
}: {
  heightClassName?: string;
}) {
  const { snapshot, error, sourceError } = useLiveData();
  const services = snapshot?.services ?? [];
  const filters = filterFor(services);
  const issue = error ?? sourceError;

  return (
    <Card className="gap-0 py-0">
      <CardContent className="p-0">
        <Tabs defaultValue="all" className="gap-0">
          <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
            <div className="min-w-0 flex-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
              <TabsList variant="line" className="h-7">
                {filters.map((filter) => (
                  <TabsTrigger
                    key={filter.value}
                    value={filter.value}
                    className="h-7 px-2.5 text-xs"
                  >
                    {filter.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>
            {issue ? (
              <Badge
                variant="outline"
                className="shrink-0 gap-1.5 rounded-none border-destructive/30 text-[10px] text-destructive"
              >
                unavailable
              </Badge>
            ) : (
              <Badge
                variant="outline"
                className="shrink-0 gap-1.5 rounded-none border-emerald-500/30 text-[10px] text-emerald-600 dark:text-emerald-400"
              >
                <Radio className="size-2.5 animate-pulse" />
                live
              </Badge>
            )}
          </div>
          {filters.map((filter) => (
            <TabsContent key={filter.value} value={filter.value} className="m-0">
              <LogTable value={filter.value} heightClassName={heightClassName} />
            </TabsContent>
          ))}
        </Tabs>
      </CardContent>
    </Card>
  );
}
