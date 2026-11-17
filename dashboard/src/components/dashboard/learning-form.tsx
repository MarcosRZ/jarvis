"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import {
  ArrowLeft,
  Check,
  Loader2,
  Save,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/dashboard/page-header";
import {
  normalizeLearningText,
  queueLearningOperation,
  targetID,
  type LearningEntry,
  type LearningEntryInput,
} from "@/lib/learning";
import { formatRelativeTime } from "@/lib/live-data";

type TargetType = "device" | "action";
type Mode = "create" | "edit" | "copy";

type FormValues = {
  text: string;
  target: TargetType;
  device: string;
  action: string;
  kind: "turn-on" | "turn-off";
  name: string;
  score: string;
  successes: string;
  failures: string;
};

type Status = {
  kind: "idle" | "saving" | "deleting" | "error";
  message: string;
};

const emptyValues: FormValues = {
  text: "",
  target: "device",
  device: "",
  action: "",
  kind: "turn-on",
  name: "",
  score: "1",
  successes: "1",
  failures: "0",
};

function valuesFromEntry(entry: LearningEntry): FormValues {
  return {
    text: entry.text,
    target: entry.action ? "action" : "device",
    device: entry.device ?? "",
    action: entry.action ?? "",
    kind: entry.kind === "turn-off" ? "turn-off" : "turn-on",
    name: entry.name ?? "",
    score: String(entry.score),
    successes: String(entry.successes),
    failures: String(entry.failures),
  };
}

function FieldHint({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-xs leading-relaxed text-muted-foreground">{children}</p>
  );
}

export function LearningForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const editText = searchParams.get("text");
  const copyText = searchParams.get("copy");
  const mode: Mode = editText ? "edit" : copyText ? "copy" : "create";
  const sourceText = editText ?? copyText;

  const [values, setValues] = useState<FormValues>(emptyValues);
  const [originalText, setOriginalText] = useState<string | null>(null);
  const [loadedEntry, setLoadedEntry] = useState<LearningEntry | null>(null);
  const [loading, setLoading] = useState(mode !== "create");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [notFound, setNotFound] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [status, setStatus] = useState<Status>({ kind: "idle", message: "" });

  useEffect(() => {
    if (!sourceText) {
      return;
    }
    let cancelled = false;

    async function load() {
      setLoading(true);
      setLoadError(null);
      setNotFound(false);
      try {
        const response = await fetch("/api/learning", { cache: "no-store" });
        const data = (await response.json()) as {
          entries?: LearningEntry[];
          error?: string;
        };
        if (!response.ok) {
          throw new Error(data.error ?? `HTTP ${response.status}`);
        }
        const entry = data.entries?.find((item) => item.text === sourceText);
        if (!entry) {
          if (!cancelled) {
            setNotFound(true);
          }
          return;
        }
        if (!cancelled) {
          setLoadedEntry(entry);
          setOriginalText(mode === "edit" ? entry.text : null);
          setValues(valuesFromEntry(entry));
        }
      } catch (loadFailure) {
        if (!cancelled) {
          setLoadError(
            loadFailure instanceof Error
              ? loadFailure.message
              : "Cannot load the association",
          );
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [sourceText, mode]);

  const normalizedText = useMemo(
    () => normalizeLearningText(values.text),
    [values.text],
  );

  function update<K extends keyof FormValues>(key: K, value: FormValues[K]) {
    setValues((previous) => ({ ...previous, [key]: value }));
    if (status.kind === "error") {
      setStatus({ kind: "idle", message: "" });
    }
  }

  function validate(): string | null {
    if (normalizedText === "") {
      return "The sentence must contain at least one letter or digit.";
    }
    if (values.target === "device" && values.device.trim() === "") {
      return "Device ID is required.";
    }
    if (values.target === "action" && values.action.trim() === "") {
      return "Action ID is required.";
    }
    if (values.score.trim() === "") {
      return "Score is required.";
    }
    if (
      !Number.isSafeInteger(Number(values.score)) ||
      !/^[+-]?\d+$/.test(values.score.trim())
    ) {
      return "Score must be a whole number.";
    }
    for (const [label, raw] of [
      ["Successes", values.successes],
      ["Failures", values.failures],
    ] as const) {
      if (raw.trim() === "" || !/^\d+$/.test(raw.trim())) {
        return `${label} must be zero or a positive whole number.`;
      }
    }
    return null;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const validation = validate();
    if (validation) {
      setStatus({ kind: "error", message: validation });
      return;
    }
    setStatus({ kind: "saving", message: "Queueing the change..." });

    const entry: LearningEntryInput = {
      text: normalizedText,
      score: Number(values.score),
      successes: Number(values.successes),
      failures: Number(values.failures),
    };
    const name = values.name.trim();
    if (name !== "") {
      entry.name = name;
    }
    if (values.target === "device") {
      entry.device = values.device.trim();
      entry.kind = values.kind;
    } else {
      entry.action = values.action.trim();
    }

    try {
      await queueLearningOperation({
        kind: "upsert",
        ...(mode === "edit" && originalText
          ? { originalText }
          : {}),
        entry,
      });
      router.push("/learning");
      router.refresh();
    } catch (saveFailure) {
      setStatus({
        kind: "error",
        message:
          saveFailure instanceof Error
            ? saveFailure.message
            : "Cannot queue the change",
      });
    }
  }

  async function remove() {
    if (!originalText) {
      return;
    }
    setStatus({ kind: "deleting", message: "Queueing the deletion..." });
    try {
      await queueLearningOperation({ kind: "delete", texts: [originalText] });
      router.push("/learning");
      router.refresh();
    } catch (deleteFailure) {
      setStatus({
        kind: "error",
        message:
          deleteFailure instanceof Error
            ? deleteFailure.message
            : "Cannot queue the deletion",
      });
    }
  }

  const title =
    mode === "edit"
      ? "Edit association"
      : mode === "copy"
        ? "Copy association"
        : "New association";
  const description =
    mode === "edit"
      ? "Change the sentence, its target or its feedback counters. Saving replaces the stored association."
      : mode === "copy"
        ? "Every field starts with the values of the original. Change the sentence before saving to create a separate association."
        : "Create a sentence-to-action association without waiting for the assistant to infer it.";
  const busy = status.kind === "saving" || status.kind === "deleting";

  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        description={description}
        badge={mode === "edit" ? "edit" : mode === "copy" ? "copy" : "new"}
      >
        <Button variant="outline" size="sm" asChild>
          <Link href="/learning">
            <ArrowLeft className="size-4" />
            Back to Learning
          </Link>
        </Button>
      </PageHeader>

      {mode === "copy" && (
        <div
          className="border border-primary/30 bg-primary/5 px-4 py-2 text-xs text-primary"
          role="status"
        >
          The original association is not changed. The store is keyed by the
          normalized sentence, so keep this sentence different or the existing
          association will be replaced.
        </div>
      )}

      {loading ? (
        <Card className="gap-0 py-0">
          <div className="space-y-4 p-6" aria-busy="true" aria-live="polite">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-9 w-2/3" />
            <Skeleton className="h-24 w-full" />
          </div>
        </Card>
      ) : loadError || notFound ? (
        <Card className="gap-0 py-0">
          <div className="flex items-start gap-3 p-6" role="alert">
            <TriangleAlert className="mt-0.5 size-5 text-destructive" />
            <div>
              <p className="text-sm font-medium">
                {notFound
                  ? "This association is no longer available."
                  : "Cannot load the association."}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                {notFound
                  ? "It may have been deleted or replaced by a newer snapshot."
                  : loadError}
              </p>
              <Button variant="outline" size="sm" className="mt-4" asChild>
                <Link href="/learning">Back to Learning</Link>
              </Button>
            </div>
          </div>
        </Card>
      ) : (
        <form onSubmit={submit} className="space-y-6">
          {status.kind === "error" && (
            <div
              className="border border-destructive/30 bg-destructive/5 px-4 py-2 text-xs text-destructive"
              role="alert"
            >
              {status.message}
            </div>
          )}

          <Card className="gap-0 py-0">
            <CardHeader className="border-b px-6 py-4">
              <CardTitle>Phrase and target</CardTitle>
            </CardHeader>
            <CardContent className="space-y-5 py-5">
              <div className="space-y-1.5">
                <label htmlFor="sentence" className="text-sm font-medium">
                  Sentence
                </label>
                <Input
                  id="sentence"
                  value={values.text}
                  onChange={(event) => update("text", event.target.value)}
                  placeholder="enciende la luz del salón"
                  maxLength={500}
                  autoComplete="off"
                  disabled={busy}
                  required
                />
                <FieldHint>
                  Stored as{" "}
                  <code className="font-mono text-foreground">
                    {normalizedText || "—"}
                  </code>
                  . The client lowercases it and removes accents, punctuation
                  and filler words.
                </FieldHint>
              </div>

              <div className="space-y-2">
                <p className="text-sm font-medium">Target type</p>
                <div
                  className="flex flex-wrap gap-1.5"
                  role="group"
                  aria-label="Target type"
                >
                  <Button
                    type="button"
                    size="sm"
                    variant={values.target === "device" ? "default" : "outline"}
                    onClick={() => update("target", "device")}
                    aria-pressed={values.target === "device"}
                    disabled={busy}
                  >
                    Device
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={values.target === "action" ? "default" : "outline"}
                    onClick={() => update("target", "action")}
                    aria-pressed={values.target === "action"}
                    disabled={busy}
                  >
                    Catalog action
                  </Button>
                </div>
              </div>

              {values.target === "device" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <label htmlFor="device" className="text-sm font-medium">
                      Device ID
                    </label>
                    <Input
                      id="device"
                      value={values.device}
                      onChange={(event) => update("device", event.target.value)}
                      placeholder="light.lounge"
                      maxLength={300}
                      autoComplete="off"
                      spellCheck={false}
                      className="font-mono text-xs"
                      disabled={busy}
                      required
                    />
                    <FieldHint>
                      The Home Assistant entity ID, for example{" "}
                      <code className="font-mono">light.lounge</code>.
                    </FieldHint>
                  </div>
                  <div className="space-y-1.5">
                    <label htmlFor="kind" className="text-sm font-medium">
                      Command
                    </label>
                    <select
                      id="kind"
                      value={values.kind}
                      onChange={(event) =>
                        update("kind", event.target.value as FormValues["kind"])
                      }
                      disabled={busy}
                      className="h-9 w-full rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-input/30"
                    >
                      <option value="turn-on">Turn on</option>
                      <option value="turn-off">Turn off</option>
                    </select>
                    <FieldHint>
                      A device association can only turn an entity on or off.
                    </FieldHint>
                  </div>
                </div>
              ) : (
                <div className="space-y-1.5">
                  <label htmlFor="action" className="text-sm font-medium">
                    Action ID
                  </label>
                  <Input
                    id="action"
                    value={values.action}
                    onChange={(event) => update("action", event.target.value)}
                    placeholder="scene.cine.turn_on"
                    maxLength={300}
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono text-xs"
                    disabled={busy}
                    required
                  />
                  <FieldHint>
                    A generated catalog action ID from the Actions page, for
                    example <code className="font-mono">scene.cine.turn_on</code>.
                  </FieldHint>
                </div>
              )}

              <div className="space-y-1.5">
                <label htmlFor="name" className="text-sm font-medium">
                  Display name <span className="font-normal text-muted-foreground">(optional)</span>
                </label>
                <Input
                  id="name"
                  value={values.name}
                  onChange={(event) => update("name", event.target.value)}
                  placeholder="la luz del salón"
                  maxLength={300}
                  autoComplete="off"
                  disabled={busy}
                />
                <FieldHint>
                  Shown in this dashboard. The assistant runs the target ID.
                </FieldHint>
              </div>
            </CardContent>
          </Card>

          <Card className="gap-0 py-0">
            <CardHeader className="border-b px-6 py-4">
              <CardTitle>Feedback counters</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 py-5">
              <div className="grid gap-4 sm:grid-cols-3">
                <div className="space-y-1.5">
                  <label htmlFor="score" className="text-sm font-medium">
                    Score
                  </label>
                  <Input
                    id="score"
                    type="number"
                    step="1"
                    value={values.score}
                    onChange={(event) => update("score", event.target.value)}
                    disabled={busy}
                    required
                  />
                </div>
                <div className="space-y-1.5">
                  <label htmlFor="successes" className="text-sm font-medium">
                    Successes
                  </label>
                  <Input
                    id="successes"
                    type="number"
                    min="0"
                    step="1"
                    value={values.successes}
                    onChange={(event) =>
                      update("successes", event.target.value)
                    }
                    disabled={busy}
                    required
                  />
                </div>
                <div className="space-y-1.5">
                  <label htmlFor="failures" className="text-sm font-medium">
                    Failures
                  </label>
                  <Input
                    id="failures"
                    type="number"
                    min="0"
                    step="1"
                    value={values.failures}
                    onChange={(event) => update("failures", event.target.value)}
                    disabled={busy}
                    required
                  />
                </div>
              </div>
              <FieldHint>
                A positive score is reused, a negative score is blocked after
                feedback and zero leaves the association inactive. Successes and
                failures are the counters shown in the table.
              </FieldHint>
            </CardContent>
          </Card>

          <Card className="gap-0 py-0">
            <CardContent className="space-y-2 py-4">
              <div className="flex items-center gap-2">
                <Badge variant="outline" className="rounded-none text-[10px]">
                  {mode === "edit" ? "Stored" : "Queued"}
                </Badge>
                <p className="text-sm font-medium">
                  {mode === "edit" && loadedEntry
                    ? targetID(loadedEntry)
                    : "The client applies this change"}
                </p>
              </div>
              <FieldHint>
                {mode === "edit" && loadedEntry
                  ? `Updated ${formatRelativeTime(
                      loadedEntry.updated,
                    )}. Changing the sentence removes the old key and stores the new one.`
                  : "The dashboard queues the change and the voice client applies it within a few seconds while it is running. Until then, the table shows the queued result."}
              </FieldHint>
            </CardContent>
          </Card>

          <div className="flex flex-wrap items-center gap-2">
            <Button type="submit" disabled={busy}>
              {status.kind === "saving" ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Save className="size-4" />
              )}
              {status.kind === "saving"
                ? "Saving..."
                : mode === "edit"
                  ? "Save changes"
                  : mode === "copy"
                    ? "Create copy"
                    : "Create association"}
            </Button>
            <Button type="button" variant="outline" asChild disabled={busy}>
              <Link href="/learning">Cancel</Link>
            </Button>

            {mode === "edit" && originalText && (
              <div className="ml-auto flex flex-wrap items-center gap-2">
                {confirmingDelete ? (
                  <>
                    <span className="text-xs text-muted-foreground">
                      Delete the stored association?
                    </span>
                    <Button
                      type="button"
                      variant="destructive"
                      onClick={remove}
                      disabled={busy}
                    >
                      {status.kind === "deleting" ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Check className="size-4" />
                      )}
                      {status.kind === "deleting"
                        ? "Deleting..."
                        : "Confirm delete"}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      onClick={() => setConfirmingDelete(false)}
                      disabled={busy}
                    >
                      Cancel
                    </Button>
                  </>
                ) : (
                  <Button
                    type="button"
                    variant="destructive"
                    onClick={() => setConfirmingDelete(true)}
                    disabled={busy}
                  >
                    <Trash2 className="size-4" />
                    Delete association
                  </Button>
                )}
              </div>
            )}
          </div>
        </form>
      )}
    </div>
  );
}
