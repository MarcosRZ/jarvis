"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import {
  Ban,
  BrainCircuit,
  Check,
  CircleCheck,
  Clock3,
  Copy,
  EyeOff,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
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
  normalizeLearningText,
  queueLearningOperation,
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

function EntryRow({
  entry,
  selected,
  confirming,
  deleting,
  onToggle,
  onRequestDelete,
  onCancelDelete,
  onConfirmDelete,
}: {
  entry: LearningEntry;
  selected: boolean;
  confirming: boolean;
  deleting: boolean;
  onToggle: () => void;
  onRequestDelete: () => void;
  onCancelDelete: () => void;
  onConfirmDelete: () => void;
}) {
  const status = learningStatus(entry);
  const editHref = `/learning/edit?text=${encodeURIComponent(entry.text)}`;
  const copyHref = `/learning/edit?copy=${encodeURIComponent(entry.text)}`;
  return (
    <TableRow className={selected ? "bg-muted/40" : undefined}>
      <TableCell className="w-10 pl-4 align-middle">
        <Checkbox
          checked={selected}
          onCheckedChange={onToggle}
          aria-label={`Select ${entry.text}`}
        />
      </TableCell>
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
      <TableCell className="text-right text-xs text-muted-foreground">
        {formatRelativeTime(entry.updated)}
      </TableCell>
      <TableCell className="pr-4">
        <div className="flex items-center justify-end gap-0.5">
          {confirming ? (
            <>
              <span className="mr-1 text-[10px] text-muted-foreground">
                Delete?
              </span>
              <Button
                variant="destructive"
                size="icon-sm"
                onClick={onConfirmDelete}
                disabled={deleting}
                aria-label={`Confirm deleting ${entry.text}`}
              >
                {deleting ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Check className="size-3.5" />
                )}
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={onCancelDelete}
                disabled={deleting}
                aria-label="Cancel delete"
              >
                <X className="size-3.5" />
              </Button>
            </>
          ) : (
            <>
              <Button
                asChild
                variant="ghost"
                size="icon-sm"
                aria-label={`Edit ${entry.text}`}
              >
                <Link href={editHref}>
                  <Pencil className="size-3.5" />
                </Link>
              </Button>
              <Button
                asChild
                variant="ghost"
                size="icon-sm"
                aria-label={`Copy ${entry.text}`}
              >
                <Link href={copyHref}>
                  <Copy className="size-3.5" />
                </Link>
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={onRequestDelete}
                aria-label={`Delete ${entry.text}`}
              >
                <X className="size-3.5" />
              </Button>
            </>
          )}
        </div>
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
            <Skeleton className="h-4 w-4" />
          </TableCell>
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
          <TableCell>
            <Skeleton className="ml-auto h-6 w-24" />
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
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [deletingText, setDeletingText] = useState<string | null>(null);
  const [bulkConfirm, setBulkConfirm] = useState(false);
  const [bulkDeleting, setBulkDeleting] = useState(false);
  const [queueError, setQueueError] = useState<string | null>(null);

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

  const availableTexts = useMemo(
    () => new Set(entries.map((entry) => entry.text)),
    [entries],
  );
  const selectedTexts = useMemo(
    () => [...selected].filter((text) => availableTexts.has(text)),
    [selected, availableTexts],
  );

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
  const pendingOperations = data?.pendingOperations ?? 0;

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

  const visibleTexts = filtered.map((entry) => entry.text);
  const allVisibleSelected =
    visibleTexts.length > 0 &&
    visibleTexts.every((text) => selected.has(text));
  const someVisibleSelected =
    !allVisibleSelected && visibleTexts.some((text) => selected.has(text));

  function toggleSelected(text: string) {
    setSelected((previous) => {
      const next = new Set(previous);
      if (next.has(text)) {
        next.delete(text);
      } else {
        next.add(text);
      }
      return next;
    });
  }

  function toggleAllVisible() {
    setSelected((previous) => {
      const next = new Set(previous);
      if (allVisibleSelected) {
        for (const text of visibleTexts) {
          next.delete(text);
        }
      } else {
        for (const text of visibleTexts) {
          next.add(text);
        }
      }
      return next;
    });
  }

  async function deleteEntries(texts: string[]): Promise<boolean> {
    setQueueError(null);
    try {
      await queueLearningOperation({
        kind: "delete",
        texts: texts.map((text) => normalizeLearningText(text) || text),
      });
      setSelected((previous) => {
        const next = new Set(previous);
        for (const text of texts) {
          next.delete(text);
        }
        return next;
      });
      setConfirmDelete(null);
      setBulkConfirm(false);
      refresh();
      return true;
    } catch (deleteError) {
      setQueueError(
        deleteError instanceof Error
          ? deleteError.message
          : "Cannot queue the deletion",
      );
      return false;
    }
  }

  async function confirmSingleDelete(text: string) {
    setDeletingText(text);
    await deleteEntries([text]);
    setDeletingText(null);
  }

  async function confirmBulkDelete() {
    setBulkDeleting(true);
    await deleteEntries(selectedTexts);
    setBulkDeleting(false);
  }

  const hasData =
    data !== null && (sourceState !== "unavailable" || entries.length > 0);
  const emptyTitle = !data
    ? "Learning data unavailable."
    : sourceState === "unavailable" && entries.length === 0
      ? "Waiting for the first snapshot."
      : entries.length === 0
        ? "No associations learned yet."
        : "No phrases match the current filters.";
  const emptyDetail = !data
    ? "Check the dashboard API and try again."
    : sourceState === "unavailable" && entries.length === 0
      ? "Start the voice assistant with VOICE_LEARNING_URL pointing at this dashboard."
      : entries.length === 0
        ? "Create one here or run a command that the assistant resolves through the catalog or the model."
        : "Clear the search or choose another feedback filter.";

  return (
    <div className="space-y-6">
      <PageHeader
        title="Learning"
        description="The sentence-to-action associations the voice assistant has learned. Create, edit or delete them here; the voice client applies the queued changes to its own store."
        badge={label.toLowerCase()}
      >
        <Button variant="outline" size="sm" asChild>
          <Link href="/learning/edit">
            <Plus className="size-4" />
            New association
          </Link>
        </Button>
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

      {pendingOperations > 0 && (
        <div
          className="flex flex-wrap items-center justify-between gap-2 border border-primary/30 bg-primary/5 px-4 py-2 text-xs text-primary"
          role="status"
          aria-live="polite"
        >
          <span>
            {pendingOperations} change
            {pendingOperations === 1 ? "" : "s"} queued for the voice client
            {data?.pendingSince
              ? `, oldest ${formatRelativeTime(data.pendingSince)}`
              : ""}
            . Changes appear here immediately and reach the client within a few
            seconds while it is running.
          </span>
          <button
            type="button"
            className="font-medium underline-offset-4 hover:underline"
            onClick={refresh}
          >
            Check now
          </button>
        </div>
      )}

      {queueError && (
        <div
          className="flex items-center justify-between gap-2 border border-destructive/30 bg-destructive/5 px-4 py-2 text-xs text-destructive"
          role="alert"
        >
          <span>Cannot queue the change: {queueError}</span>
          <button
            type="button"
            className="font-medium underline-offset-4 hover:underline"
            onClick={() => setQueueError(null)}
          >
            Dismiss
          </button>
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
            dashboard and run the assistant. Changes created here wait in the
            dashboard queue until it connects.
          </p>
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Learned phrases"
          value={hasData ? String(entries.length) : unknownValue}
          detail="normalized whole sentences"
          icon={BrainCircuit}
        />
        <StatCard
          label="Active"
          value={hasData ? String(active) : unknownValue}
          detail="will be reused"
          icon={CircleCheck}
        />
        <StatCard
          label="Penalized"
          value={hasData ? String(penalized) : unknownValue}
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

      {selectedTexts.length > 0 && (
        <div
          className="flex flex-wrap items-center gap-2 border bg-muted/40 px-4 py-2"
          role="region"
          aria-label="Selected associations"
        >
          <span className="text-sm font-medium tabular-nums">
            {selectedTexts.length} selected
          </span>
          {bulkConfirm ? (
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-xs text-muted-foreground">
                Delete {selectedTexts.length}{" "}
                {selectedTexts.length === 1 ? "association" : "associations"}?
              </span>
              <Button
                variant="destructive"
                size="sm"
                onClick={confirmBulkDelete}
                disabled={bulkDeleting}
              >
                {bulkDeleting ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Trash2 className="size-4" />
                )}
                {bulkDeleting ? "Deleting..." : "Confirm"}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setBulkConfirm(false)}
                disabled={bulkDeleting}
              >
                Cancel
              </Button>
            </div>
          ) : (
            <div className="flex flex-wrap items-center gap-1.5">
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setBulkConfirm(true)}
              >
                <Trash2 className="size-4" />
                Delete
              </Button>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setSelected(new Set())}
              >
                Clear selection
              </Button>
            </div>
          )}
        </div>
      )}

      <Card className="gap-0 overflow-hidden py-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10 pl-4 align-middle">
                <Checkbox
                  checked={
                    allVisibleSelected
                      ? true
                      : someVisibleSelected
                        ? "indeterminate"
                        : false
                  }
                  onCheckedChange={toggleAllVisible}
                  aria-label={
                    allVisibleSelected
                      ? "Clear selection"
                      : "Select all visible associations"
                  }
                />
              </TableHead>
              <TableHead>Phrase</TableHead>
              <TableHead className="hidden md:table-cell">Target</TableHead>
              <TableHead>Feedback</TableHead>
              <TableHead className="text-right">Updated</TableHead>
              <TableHead className="pr-4 text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableSkeleton />
            ) : (
              filtered.map((entry) => (
                <EntryRow
                  key={entry.text}
                  entry={entry}
                  selected={selected.has(entry.text)}
                  confirming={confirmDelete === entry.text}
                  deleting={deletingText === entry.text}
                  onToggle={() => toggleSelected(entry.text)}
                  onRequestDelete={() => setConfirmDelete(entry.text)}
                  onCancelDelete={() => setConfirmDelete(null)}
                  onConfirmDelete={() => confirmSingleDelete(entry.text)}
                />
              ))
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
              Queue
            </Badge>
            <p className="text-sm font-medium">
              The client owns the learning store
            </p>
          </div>
          <p className="mt-2 max-w-3xl text-xs leading-relaxed text-muted-foreground">
            The dashboard stores the last snapshot and a queue of changes in the{" "}
            <code className="font-mono">dashboard-data</code> volume with mode{" "}
            <code className="font-mono">0600</code>. The voice client polls the
            queue, applies each change to its local store and publishes a new
            snapshot. Changes stay queued while the client is stopped, and the
            dashboard cannot check whether a target still exists in Home
            Assistant.
          </p>
        </div>
      </Card>
    </div>
  );
}
