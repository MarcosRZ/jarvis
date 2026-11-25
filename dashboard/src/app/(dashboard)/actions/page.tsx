"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Check,
  CircleAlert,
  Clock3,
  Layers,
  ListChecks,
  Loader2,
  Play,
  RefreshCw,
  Search,
  ShieldCheck,
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
import { useActions } from "@/components/dashboard/use-actions";
import type { CatalogAction } from "@/lib/actions";
import { formatRelativeTime, unknownValue } from "@/lib/live-data";

const sourceLabel: Record<string, string> = {
  ok: "Live",
  degraded: "Stale",
  unavailable: "Unavailable",
  disabled: "Disabled",
};

const runErrors: Record<string, string> = {
  "action not found": "the catalog changed; refresh and try again",
  "action catalog unavailable": "the catalog is unavailable",
  "action failed": "Home Assistant did not run it",
  action_not_found: "the catalog changed; refresh and try again",
  action_failed: "Home Assistant did not run it",
  action_catalog_unavailable: "the catalog is unavailable",
  collector_unavailable: "the collector is unreachable",
  collector_timeout: "the collector did not answer in time",
  invalid_action_request: "the request was invalid",
  action_required: "the request had no action",
  invalid_json: "the request was invalid",
};

type RunState = "idle" | "running" | "done" | "failed";

type RunResult = {
  message: string;
  failed: boolean;
};

function ActionRow({
  action,
  onResult,
}: {
  action: CatalogAction;
  onResult: (result: RunResult) => void;
}) {
  const data = action.data ? Object.entries(action.data) : [];
  const [runState, setRunState] = useState<RunState>("idle");

  // A finished run shows its icon briefly, then returns to the play button.
  useEffect(() => {
    if (runState !== "done" && runState !== "failed") {
      return;
    }
    const timer = setTimeout(
      () => setRunState("idle"),
      runState === "done" ? 2000 : 5000,
    );
    return () => clearTimeout(timer);
  }, [runState]);

  async function run() {
    if (runState === "running") {
      return;
    }
    setRunState("running");
    try {
      const response = await fetch("/api/actions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id: action.id }),
      });
      const result = (await response.json().catch(() => null)) as {
        error?: string;
        name?: string;
      } | null;
      if (!response.ok) {
        const code = result?.error ?? "";
        throw new Error(runErrors[code] ?? `HTTP ${response.status}`);
      }
      setRunState("done");
      onResult({
        message: `Ran ${result?.name ?? action.name}.`,
        failed: false,
      });
    } catch (error) {
      setRunState("failed");
      onResult({
        message: `Could not run ${action.name}: ${
          error instanceof Error ? error.message : "unknown error"
        }.`,
        failed: true,
      });
    }
  }

  return (
    <TableRow>
      <TableCell className="whitespace-normal py-3">
        <p className="font-medium">{action.name}</p>
        <p className="mt-0.5 font-mono text-[10px] text-muted-foreground">
          {action.id}
        </p>
      </TableCell>
      <TableCell className="hidden md:table-cell">
        <span className="font-mono text-xs">{action.entity_id}</span>
      </TableCell>
      <TableCell className="hidden lg:table-cell">
        <span className="font-mono text-xs">{action.service}</span>
        {data.length > 0 && (
          <span className="ml-2 font-mono text-[10px] text-muted-foreground">
            {data.map(([key, value]) => `${key}=${String(value)}`).join(" ")}
          </span>
        )}
      </TableCell>
      <TableCell className="text-right md:text-left">
        <Badge variant="outline" className="rounded-none text-[10px]">
          {action.domain}
        </Badge>
      </TableCell>
      <TableCell className="pr-4 text-right">
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="size-8"
          onClick={run}
          disabled={runState === "running"}
          aria-label={`Run ${action.name}`}
          title={`Run ${action.name}`}
        >
          {runState === "running" ? (
            <Loader2 className="size-4 animate-spin" />
          ) : runState === "done" ? (
            <Check className="size-4 text-emerald-600 dark:text-emerald-400" />
          ) : runState === "failed" ? (
            <CircleAlert className="size-4 text-destructive" />
          ) : (
            <Play className="size-4" />
          )}
        </Button>
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
            <Skeleton className="h-4 w-40" />
            <Skeleton className="mt-1.5 h-2.5 w-52" />
          </TableCell>
          <TableCell className="hidden md:table-cell">
            <Skeleton className="h-3 w-32" />
          </TableCell>
          <TableCell className="hidden lg:table-cell">
            <Skeleton className="h-3 w-28" />
          </TableCell>
          <TableCell>
            <Skeleton className="ml-auto h-4 w-14 md:ml-0" />
          </TableCell>
          <TableCell className="pr-4">
            <Skeleton className="ml-auto size-8" />
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
  icon: typeof ListChecks;
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

export default function ActionsPage() {
  const { data, loading, refreshing, error, stale, refresh } = useActions();
  const [query, setQuery] = useState("");
  const [domain, setDomain] = useState<string | null>(null);
  const [runResult, setRunResult] = useState<RunResult | null>(null);

  const actions = useMemo(() => data?.actions ?? [], [data]);
  const source = data?.source ?? null;
  const sourceState = source?.state ?? (error ? "unavailable" : "disabled");
  const label = loading ? "Loading" : (sourceLabel[sourceState] ?? "Unknown");

  const domains = useMemo(() => {
    const counts = new Map<string, number>();
    for (const action of actions) {
      counts.set(action.domain, (counts.get(action.domain) ?? 0) + 1);
    }
    return [...counts.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [actions]);

  const normalizedQuery = query.trim().toLowerCase();
  const filtered = useMemo(() => {
    return actions.filter((action) => {
      if (domain && action.domain !== domain) {
        return false;
      }
      if (!normalizedQuery) {
        return true;
      }
      return [
        action.name,
        action.id,
        action.entity_id,
        action.service,
        action.domain,
        action.description,
      ].some((value) => value.toLowerCase().includes(normalizedQuery));
    });
  }, [actions, domain, normalizedQuery]);

  const issue = error ?? (sourceState === "unavailable" ? source?.message : null);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Actions"
        description="The deterministic catalog the voice assistant can run right now, generated from the entities exposed to Home Assistant. The play button runs one action now."
        badge={label.toLowerCase()}
      >
        <Button
          variant="outline"
          size="sm"
          onClick={refresh}
          disabled={refreshing}
          aria-label="Refresh action catalog"
        >
          <RefreshCw className={refreshing ? "size-4 animate-spin" : "size-4"} />
          Refresh
        </Button>
      </PageHeader>

      {runResult && (
        <p
          role="status"
          aria-live="polite"
          className={
            runResult.failed
              ? "border border-destructive/30 bg-destructive/5 px-4 py-2 text-xs text-destructive"
              : "border border-emerald-500/30 bg-emerald-500/5 px-4 py-2 text-xs text-emerald-700 dark:text-emerald-400"
          }
        >
          {runResult.message}
        </p>
      )}

      {(stale || (error && data !== null)) && (
        <div
          className="flex items-center justify-between gap-2 border border-amber-500/30 bg-amber-500/5 px-4 py-2 text-xs text-amber-700 dark:text-amber-400"
          role="status"
          aria-live="polite"
        >
          <span>
            Showing the last catalog received. The collector is unreachable:
            {error}
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

      {sourceState === "degraded" && !stale && (
        <div
          className="border border-amber-500/30 bg-amber-500/5 px-4 py-2 text-xs text-amber-700 dark:text-amber-400"
          role="status"
          aria-live="polite"
        >
          Home Assistant could not be reached. Showing the last catalog from{" "}
          {formatRelativeTime(data?.generatedAt ?? null)}.
          {source?.message ? ` ${source.message}` : ""}
        </div>
      )}

      {sourceState === "unavailable" && !stale && (
        <div
          className="flex items-center justify-between gap-2 border border-destructive/30 bg-destructive/5 px-4 py-2 text-xs text-destructive"
          role="status"
          aria-live="polite"
        >
          <span>
            The action catalog is unavailable.
            {issue ? ` ${issue}` : ""}
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

      {sourceState === "disabled" && !loading && (
        <div
          className="border border-border bg-muted/30 px-4 py-3 text-xs text-muted-foreground"
          role="status"
        >
          <p className="font-medium text-foreground">Action catalog disabled</p>
          <p className="mt-1 leading-relaxed">
            Set <code className="font-mono">VOICE_HA_URL</code> and{" "}
            <code className="font-mono">VOICE_HA_TOKEN</code> in{" "}
            <code className="font-mono">server/.env</code>, then restart the
            collector. The collector reads the same exposure settings as the
            voice client.
          </p>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Actions"
          value={loading ? unknownValue : String(actions.length)}
          detail="selectable from the catalog"
          icon={ListChecks}
        />
        <StatCard
          label="Exposed entities"
          value={loading ? unknownValue : String(data?.entityCount ?? 0)}
          detail="visible to the assistant"
          icon={ShieldCheck}
        />
        <StatCard
          label="Domains"
          value={loading ? unknownValue : String(domains.length)}
          detail="light, switch, media_player..."
          icon={Layers}
        />
        <StatCard
          label="Updated"
          value={loading ? unknownValue : formatRelativeTime(data?.generatedAt ?? null)}
          detail="catalog refresh interval: 30s"
          icon={Clock3}
        />
      </div>

      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="relative w-full max-w-sm">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search actions, entities or services..."
            className="pl-9"
            aria-label="Search actions"
          />
        </div>
        <div className="flex flex-wrap items-center gap-1.5" aria-label="Filter by domain">
          <Button
            variant={domain === null ? "default" : "outline"}
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={() => setDomain(null)}
            aria-pressed={domain === null}
          >
            All
            <span className="ml-1 tabular-nums opacity-70">{actions.length}</span>
          </Button>
          {domains.map(([name, count]) => (
            <Button
              key={name}
              variant={domain === name ? "default" : "outline"}
              size="sm"
              className="h-7 px-2 text-xs"
              onClick={() => setDomain(domain === name ? null : name)}
              aria-pressed={domain === name}
            >
              {name}
              <span className="ml-1 tabular-nums opacity-70">{count}</span>
            </Button>
          ))}
        </div>
      </div>

      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="pl-4">Action</TableHead>
              <TableHead className="hidden md:table-cell">Entity</TableHead>
              <TableHead className="hidden lg:table-cell">Service</TableHead>
              <TableHead className="text-right md:text-left">Domain</TableHead>
              <TableHead className="w-12 pr-4 text-right">
                <span className="sr-only">Run</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableSkeleton />
            ) : (
              filtered.map((action) => (
                <ActionRow
                  key={action.id}
                  action={action}
                  onResult={setRunResult}
                />
              ))
            )}
          </TableBody>
        </Table>
        {!loading && filtered.length === 0 && (
          <div className="border-t px-4 py-8 text-center" role="status">
            <p className="text-sm font-medium">
              {actions.length === 0
                ? "No exposed entity produces an action."
                : "No actions match the current filters."}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {actions.length === 0
                ? "Expose controllable entities in Home Assistant, or check the source configuration above."
                : "Clear the search or choose another domain."}
            </p>
          </div>
        )}
      </Card>

      {!loading && (query || domain) && filtered.length > 0 && (
        <p className="text-xs text-muted-foreground" aria-live="polite">
          Showing {filtered.length} of {actions.length} actions.
        </p>
      )}

      <Card className="gap-0 py-0">
        <div className="p-4">
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="rounded-none text-[10px]">
              Scope
            </Badge>
            <p className="text-sm font-medium">What this list covers</p>
          </div>
          <p className="mt-2 max-w-3xl text-xs leading-relaxed text-muted-foreground">
            This page lists only the deterministic action catalog. Built-in
            intents such as time, repeat, cancel, help and feedback, learned
            phrases and the Home Assistant conversation agent are handled
            separately and are not part of this list. Changes to the exposed
            entities can take up to 30 seconds to appear here. The play button
            calls the fixed Home Assistant service of that action; running it
            here does not change what the voice assistant has learned.
          </p>
        </div>
      </Card>
    </div>
  );
}
