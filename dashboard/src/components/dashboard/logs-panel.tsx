"use client";

import { Radio, ScrollText } from "lucide-react";
import { cn } from "cn";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { logs } from "@/lib/mock-data";

const levelStyles: Record<string, string> = {
  info: "border-primary/30 text-primary",
  warn: "border-amber-500/30 text-amber-600 dark:text-amber-400",
  error: "border-destructive/30 text-destructive",
  debug: "border-border text-muted-foreground",
};

const filters = [
  { value: "all", label: "Todos" },
  { value: "errors", label: "Errores" },
  { value: "whisper", label: "whisper" },
  { value: "llama-server", label: "llama" },
  { value: "piper", label: "piper" },
];

function filtered(value: string) {
  if (value === "all") return logs;
  if (value === "errors") return logs.filter((log) => log.level === "error");
  return logs.filter((log) => log.service === value);
}

function LogTable({
  value,
  heightClassName,
}: {
  value: string;
  heightClassName: string;
}) {
  const entries = filtered(value);
  return (
    <ScrollArea className={heightClassName}>
      <Table>
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
                {log.time}
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
              <TableCell className="max-w-[520px] truncate py-2">
                {log.message}
              </TableCell>
            </TableRow>
          ))}
          {entries.length === 0 && (
            <TableRow>
              <TableCell
                colSpan={4}
                className="py-10 text-center text-xs text-muted-foreground"
              >
                Sin registros para este filtro.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </ScrollArea>
  );
}

export function LogsPanel({
  heightClassName = "h-[320px]",
}: {
  heightClassName?: string;
}) {
  return (
    <Card className="gap-0 py-0">
      <CardHeader className="border-b p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <ScrollText className="size-4 text-primary" />
            <CardTitle className="text-sm">Registros</CardTitle>
          </div>
          <Badge
            variant="outline"
            className="gap-1.5 rounded-none border-emerald-500/30 text-[10px] text-emerald-600 dark:text-emerald-400"
          >
            <Radio className="size-2.5 animate-pulse" />
            live
          </Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          Ultimas entradas de todos los contenedores
        </p>
      </CardHeader>
      <CardContent className="p-0">
        <Tabs defaultValue="all" className="gap-0">
          <div className="border-b px-3 py-2">
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
          {filters.map((filter) => (
            <TabsContent key={filter.value} value={filter.value} className="m-0">
              <LogTable
                value={filter.value}
                heightClassName={heightClassName}
              />
            </TabsContent>
          ))}
        </Tabs>
      </CardContent>
    </Card>
  );
}
