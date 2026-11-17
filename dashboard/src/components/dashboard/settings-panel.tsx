"use client";

import { useEffect, useState } from "react";
import { Eye, EyeOff, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { PromptSettings } from "@/components/dashboard/prompt-settings";
import {
  settingsSections,
  validateSettingValue,
  type SettingField,
} from "@/lib/settings-schema";

type Status = {
  kind: "idle" | "saved" | "error";
  message: string;
};

const selectClassName =
  "h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30";

function SettingInput({
  field,
  value,
  onChange,
  onReset,
}: {
  field: SettingField;
  value: string;
  onChange: (value: string) => void;
  onReset: () => void;
}) {
  const [revealed, setRevealed] = useState(false);
  const overridden = value !== "";

  return (
    <div className={field.wide ? "space-y-2 sm:col-span-2" : "space-y-2"}>
      <div className="flex items-center justify-between gap-2">
        <label
          htmlFor={`setting-${field.key}`}
          className="text-sm font-medium"
        >
          {field.label}
        </label>
        <div className="flex items-center gap-1.5">
          {overridden ? (
            <Badge
              variant="outline"
              className="rounded-none border-primary/30 px-1.5 py-0 text-[9px] uppercase tracking-wide text-primary"
            >
              override
            </Badge>
          ) : (
            <span className="text-[10px] text-muted-foreground">Using .env</span>
          )}
          {overridden && (
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={onReset}
              aria-label={`Remove the ${field.label} override`}
              title="Remove override"
            >
              <X className="size-3" />
            </Button>
          )}
        </div>
      </div>
      {field.type === "textarea" ? (
        <Textarea
          id={`setting-${field.key}`}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={field.placeholder}
          maxLength={field.maxLength}
          rows={3}
          className="text-sm"
        />
      ) : field.type === "select" ? (
        <select
          id={`setting-${field.key}`}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className={selectClassName}
        >
          <option value="">Using .env</option>
          {field.options?.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      ) : field.type === "password" ? (
        <div className="relative">
          <Input
            id={`setting-${field.key}`}
            type={revealed ? "text" : "password"}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            placeholder={field.placeholder}
            maxLength={field.maxLength}
            className="pr-9"
          />
          <Button
            variant="ghost"
            size="icon-xs"
            className="absolute right-1 top-1/2 -translate-y-1/2"
            onClick={() => setRevealed((current) => !current)}
            aria-label={revealed ? "Hide value" : "Show value"}
            type="button"
          >
            {revealed ? (
              <EyeOff className="size-3.5" />
            ) : (
              <Eye className="size-3.5" />
            )}
          </Button>
        </div>
      ) : (
        <Input
          id={`setting-${field.key}`}
          type={field.type === "number" ? "number" : "text"}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={field.placeholder}
          min={field.min}
          max={field.max}
          step={field.step}
          maxLength={field.maxLength}
          className="text-sm"
        />
      )}
      <p className="text-[10px] leading-relaxed text-muted-foreground">
        <code className="font-mono">{field.key}</code> · {field.description}
      </p>
    </div>
  );
}

export function SettingsPanel() {
  const [tab, setTab] = useState("general");
  const [values, setValues] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [status, setStatus] = useState<Status>({ kind: "idle", message: "" });

  useEffect(() => {
    let cancelled = false;
    fetch("/api/settings", { cache: "no-store" })
      .then(async (response) => {
        const data = (await response.json()) as {
          values?: Record<string, string>;
          error?: string;
        };
        if (!response.ok) {
          throw new Error(data.error ?? `HTTP ${response.status}`);
        }
        if (!cancelled) {
          setValues(data.values ?? {});
          setLoading(false);
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setStatus({
            kind: "error",
            message:
              error instanceof Error ? error.message : "Cannot load settings",
          });
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function updateValue(key: string, value: string) {
    setValues((current) => {
      const next = { ...current };
      if (value === "") {
        delete next[key];
      } else {
        next[key] = value;
      }
      return next;
    });
    setDirty(true);
    setStatus({ kind: "idle", message: "" });
  }

  async function save() {
    for (const section of settingsSections) {
      for (const field of section.fields) {
        const value = values[field.key];
        if (!value) {
          continue;
        }
        const problem = validateSettingValue(field, value);
        if (problem) {
          setStatus({ kind: "error", message: problem });
          setTab(section.id);
          return;
        }
      }
    }
    setSaving(true);
    try {
      const response = await fetch("/api/settings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ values }),
      });
      const data = (await response.json()) as {
        values?: Record<string, string>;
        error?: string;
      };
      if (!response.ok) {
        throw new Error(data.error ?? `HTTP ${response.status}`);
      }
      setValues(data.values ?? {});
      setDirty(false);
      setStatus({
        kind: "saved",
        message:
          "Saved. Run make services-up to apply the server and shared values, and restart make run for client values.",
      });
    } catch (error) {
      setStatus({
        kind: "error",
        message: error instanceof Error ? error.message : "Cannot save settings",
      });
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <Card className="gap-0 py-0">
        <CardHeader className="border-b p-5">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-80" />
        </CardHeader>
        <CardContent className="space-y-4 p-5">
          <Skeleton className="h-9 w-full max-w-xl" />
          <Skeleton className="h-9 w-full max-w-xl" />
          <Skeleton className="h-9 w-full max-w-xl" />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card className="gap-0 py-0">
        <CardHeader className="border-b p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="space-y-1">
              <h2 className="text-sm font-semibold">Configuration overrides</h2>
              <p className="text-xs text-muted-foreground">
                Saved values override the .env files. Empty fields keep using
                .env. The Home Assistant URL and token are shared by the
                collector and the voice client.
              </p>
            </div>
            <Button onClick={save} disabled={saving || !dirty}>
              {saving ? "Saving..." : "Save overrides"}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Tabs value={tab} onValueChange={setTab} className="gap-0">
            <div className="border-b px-3 py-2">
              <div className="overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
                <TabsList variant="line" className="h-8">
                  {settingsSections.map((section) => (
                    <TabsTrigger
                      key={section.id}
                      value={section.id}
                      className="h-8 px-2.5 text-xs"
                    >
                      {section.title}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </div>
            </div>
            {settingsSections.map((section) => (
              <TabsContent
                key={section.id}
                value={section.id}
                className="m-0 p-5"
              >
                <p className="mb-4 text-xs text-muted-foreground">
                  {section.description}
                </p>
                <div className="grid gap-4 sm:grid-cols-2">
                  {section.fields.map((field) => (
                    <SettingInput
                      key={field.key}
                      field={field}
                      value={values[field.key] ?? ""}
                      onChange={(value) => updateValue(field.key, value)}
                      onReset={() => updateValue(field.key, "")}
                    />
                  ))}
                </div>
              </TabsContent>
            ))}
          </Tabs>
        </CardContent>
        {status.message && (
          <div className="border-t px-5 py-3">
            <p
              className={
                status.kind === "error"
                  ? "text-xs text-destructive"
                  : "text-xs text-muted-foreground"
              }
              role="status"
            >
              {status.message}
            </p>
          </div>
        )}
      </Card>

      {tab === "general" && <PromptSettings />}
    </div>
  );
}
