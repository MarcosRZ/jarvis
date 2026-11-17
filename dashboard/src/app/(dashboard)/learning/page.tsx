"use client";

import { useMemo, useState } from "react";
import {
  Ban,
  BrainCircuit,
  CircleCheck,
  Clock3,
  EyeOff,
  RefreshCw,
  Search,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { PageHeader } from "@/components/dashboard/page-header";
import { useLearning } from "@/components/dashboard/use-learning";
import {
  learningStatus,
  targetID,
  type LearningEntry,
} from "@/lib/learning";
import { formatRelativeTime, unknownValue } from "@/lib/live-data";

const sourceLabel: Record<string, string> = {
  ok: "Live",
  degraded: "Stale",
  unavailable: "Unavailable",
  disabled: "Disabled",
};

type Filter = "all" | "active" | "penalized";

function EntryRow({ entry }: { entry: LearningEntry }) {
  const status = learningStatus(entry);
  return (
    <TableRow>
      <TableCell className="whitespace-normal py-3">
        <p className="font-medium">{entry.text}</p>
      </TableCell>
      <TableCell className="hidden md:table-cell">
        {entry.name && <p className="text-sm">{entry.name}</p>}
        <p className="font-mono text-[10px] text-muted-foreground">
          {targetID(entry)}
          {entry.kind ? ` · ${entry.kind}` : ""}
        </p>
      </TableCell>
      <TableCell>
        <Badge
          variant={status === "penalized" ? "destructive" : "outline"}
          className="rounded-none text-[10px]"
        >
          {status === "active"
            ? "Active"
            : status === "penalized"
              ? "Penalized"
              : "Inactive"}
        </Badge>
        <p className="mt-1 text-[10px] tabular-nums text-muted-foreground">
          score {entry.score} · {entry.successes} ok · {entry.failures} failed
        </p>
      </TableCell>
      <TableCell className="pr-4 text-right text-xs text-muted-foreground">
        {formatRelativeTime(entry.updated)}
      </TableCell>
    </TableRow>
  );
}

function TableSkeleton() {
  return (
    <>
      {Array.from({ length: 6 }, (_, index) => (
        <TableRow key={index}>
          <TableCell className="py-3">
            <Skeleton className="h-4 w-44" />
          </TableCell>
          <TableCell className="hidden md:table-cell">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="mt-1.5 h-2.5 w-40" />
          </TableCell>
          <TableCell>
            <Skeleton className="h-4 w-20" />
            <Skeleton className="mt-1.5 h-2.5 w-28" />
          </TableCell>
          <TableCell>
            <Skeleton className="ml-auto h-3 w-16" />
          </TableCell>
        </TableRow>
      ))}
    </>
  );
}

function StatCard({
  label,
  value,
  detail,
  icon: Icon,
}: {
  label: string;
  value: string;
  detail: string;
  icon: typeof BrainCircuit;
}) {
  return (
    <Card className="gap-0 py-0">
      <div className="flex items-start justify-between p-4">
        <div>
          <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            {label}
          </p>
          <p className="mt-2 text-2xl font-semibold tabular-nums">{value}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">{detail}</p>
        </div>
        <div className="flex size-9 items-center justify-center border bg-muted/40">
          <Icon className="size-4 text-primary" />
        </div>
      </div>
    </Card>
  );
}

export default function LearningPage() {
  const { data, loading, refreshing, error, stale, refresh } = useLearning();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");

  const entries = useMemo(() => {
    return [...(data?.entries ?? [])].sort((a, b) => {
      const left = Date.parse(b.updated);
      const right = Date.parse(a.updated);
      if (Number.isNaN(left) || Number.isNaN(right)) {
        return a.text.localeCompare(b.text);
      }
      return left - right || a.text.localeCompare(b.text);
    });
  }, [data]);

  const active = entries.filter(
    (entry) => learningStatus(entry) === "active",
  ).length;
  const penalized = entries.filter(
    (entry) => learningStatus(entry) === "penalized",
  ).length;
  const source = data?.source ?? null;
  const sourceState = source?.state ?? (error ? "unavailable" : "disabled");
  const label = loading
    ? "Loading"
    : stale
      ? "Stale"
      : (sourceLabel[sourceState] ?? "Unknown");

  const normalizedQuery = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    return entries.filter((entry) => {
      if (filter === "active" && learningStatus(entry) !== "active") {
        return false;
      }
      if (filter === "penalized" && learningStatus(entry) !== "penalized") {
        return false;
      }
      if (!normalizedQuery) {
        return true;
      }
      return [entry.text, entry.name, entry.device, entry.action, entry.kind]
        .filter((value): value is string => typeof value === "string")
        .some((value) => value.toLowerCase().includes(normalizedQuery));
    });
  }, [entries, filter, normalizedQuery]);

  const hasSnapshot = data !== null && sourceState !== "unavailable";
  const emptyTitle = !data
    ? "Learning data unavailable."
    : sourceState === "unavailable"
      ? "Waiting for the first snapshot."
      : entries.length === 0
        ? "No associations learned yet."
        : "No phrases match the current filters.";
  const emptyDetail = !data
    ? "Check the dashboard API and try again."
    : sourceState === "unavailable"
      ? "Start the voice assistant with VOICE_LEARNING_URL pointing at this dashboard."
      : entries.length === 0
        ? "A successful command that resolves through the catalog or the model is learned after it runs."
        : "Clear the search or choose another feedback filter.";

  return (
    <div className="space-y-6">
      <PageHeader
        title="Learning"
        description="The sentence-to-action associations the voice assistant has learned and still uses, published as a read-only snapshot by the client."
        badge={label.toLowerCase()}
      >
        <Button
          variant="outline"
          size="sm"
          onClick={refresh}
          disabled={refreshing}
          aria-label="Refresh learned associations"
        >
          <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
          Refresh
        </Button>
      </PageHeader>

      <Card className="gap-0 py-0">
        <div className="flex gap-3 p-4">
          <EyeOff className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
          <div>
            <p className="text-sm font-medium">Recognized speech is private</p>
            <p className="mt-1 max-w-3xl text-xs leading-relaxed text-muted-foreground">
              The phrases below are what the assistant heard. The dashboard has
              no authentication, so keep it on localhost or a trusted network.
            </p>
          </div>
        </div>
      </Card>

      {stale && (
        <div
          className="flex items-center justify-between gap-2 border border-amber-500/30 bg-amber-500/5 px-4 py-2 text-xs text-amber-700 dark:text-amber-400"
          role="status"
          aria-live="polite"
        >
          <span>
            Showing the last snapshot received. The dashboard API is
            unreachable: {error}
          </span>
          <button
            type="button"
            className="font-medium underline-offset-4 hover:underline"
            onClick={refresh}
          >
            Retry
          </button>
        </div>
      )}

      {error && data === null && (
        <div
          className="flex items-center justify-between gap-2 border border-destructive/30 bg-destructive/5 px-4 py-2 text-xs text-destructive"
          role="status"
          aria-live="polite"
        >
          <span>The learning snapshot is unavailable: {error}</span>
          <button
            type="button"
            className="font-medium underline-offset-4 hover:underline"
            onClick={refresh}
          >
            Retry
          </button>
        </div>
      )}

      {sourceState === "degraded" && !stale && (
        <div
          className="border border-amber-500/30 bg-amber-500/5 px-4 py-2 text-xs text-amber-700 dark:text-amber-400"
          role="status"
          aria-live="polite"
        >
          {source?.message ??
            "The voice client has not published a snapshot recently."}{" "}
          Showing the last one from{" "}
          {formatRelativeTime(data?.receivedAt ?? null)}.
        </div>
      )}

      {sourceState === "unavailable" && !loading && data !== null && (
        <div
          className="border border-border bg-muted/30 px-4 py-3 text-xs text-muted-foreground"
          role="status"
        >
          <p className="font-medium text-foreground">No snapshot yet</p>
          <p className="mt-1 max-w-3xl leading-relaxed">
            The voice client publishes learned associations to{" "}
            <code className="font-mono">POST /api/learning</code> at startup,
            after a change and every 60 seconds. Keep{" "}
            <code className="font-mono">VOICE_LEARNING_URL</code> in{" "}
            <code className="font-mono">client/.env</code> pointed at this
            dashboard and run the assistant.
          </p>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Learned phrases"
          value={hasSnapshot ? String(entries.length) : unknownValue}
          detail="normalized whole sentences"
          icon={BrainCircuit}
        />
        <StatCard
          label="Active"
          value={hasSnapshot ? String(active) : unknownValue}
          detail="will be reused"
          icon={CircleCheck}
        />
        <StatCard
          label="Penalized"
          value={hasSnapshot ? String(penalized) : unknownValue}
          detail="blocked by feedback"
          icon={Ban}
        />
        <StatCard
          label="Last publish"
          value={
            loading ? unknownValue : formatRelativeTime(data?.receivedAt ?? null)
          }
          detail="snapshot heartbeat: 60s"
          icon={Clock3}
        />
      </div>

      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="relative w-full max-w-sm">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search phrases, targets or actions..."
            className="pl-9"
            aria-label="Search learned associations"
          />
        </div>
        <div className="flex flex-wrap items-center gap-1.5" aria-label="Filter by feedback">
          <Button
            variant={filter === "all" ? "default" : "outline"}
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={() => setFilter("all")}
            aria-pressed={filter === "all"}
          >
            All
            <span className="ml-1 tabular-nums opacity-70">{entries.length}</span>
          </Button>
          <Button
            variant={filter === "active" ? "default" : "outline"}
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={() => setFilter("active")}
            aria-pressed={filter === "active"}
          >
            Active
            <span className="ml-1 tabular-nums opacity-70">{active}</span>
          </Button>
          <Button
            variant={filter === "penalized" ? "default" : "outline"}
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={() => setFilter("penalized")}
            aria-pressed={filter === "penalized"}
          >
            Penalized
            <span className="ml-1 tabular-nums opacity-70">{penalized}</span>
          </Button>
        </div>
      </div>

      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-4">Phrase</TableHead>
              <TableHead className="hidden md:table-cell">Target</TableHead>
              <TableHead>Feedback</TableHead>
              <TableHead className="pr-4 text-right">Updated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableSkeleton />
            ) : (
              filtered.map((entry) => <EntryRow key={entry.text} entry={entry} />)
            )}
          </TableBody>
        </Table>
        {!loading && filtered.length === 0 && (
          <div className="border-t px-4 py-8 text-center" role="status">
            <p className="text-sm font-medium">{emptyTitle}</p>
            <p className="mt-1 text-xs text-muted-foreground">{emptyDetail}</p>
          </div>
        )}
      </Card>

      {!loading && (query || filter !== "all") && filtered.length > 0 && (
        <p className="text-xs text-muted-foreground" aria-live="polite">
          Showing {filtered.length} of {entries.length} associations.
        </p>
      )}

      <Card className="gap-0 py-0">
        <div className="p-4">
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="rounded-none text-[10px]">
              Mirror
            </Badge>
            <p className="text-sm font-medium">A read-only view of client memory</p>
          </div>
          <p className="mt-2 max-w-3xl text-xs leading-relaxed text-muted-foreground">
            The dashboard stores the last snapshot in the{" "}
            <code className="font-mono">dashboard-data</code> volume with mode{" "}
            <code className="font-mono">0600</code>. It does not run or change
            learning, and it cannot check whether a target still exists in Home
            Assistant. A stopped client, or publishing turned off, leaves the
            last snapshot visible until the heartbeat expires.
          </p>
        </div>
      </Card>
    </div>
  );
}
