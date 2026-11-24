"use client";

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Status = {
  kind: "idle" | "saved" | "error" | "loading";
  message: string;
};

type ModelSettings = {
  url: string;
  model: string;
  timeout: string;
  configured: boolean;
};

export function ModelSettingsPanel({ onSaved }: { onSaved?: () => void }) {
  const [values, setValues] = useState<ModelSettings>({
    url: "",
    model: "",
    timeout: "",
    configured: false,
  });
  const [apiKey, setApiKey] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState<Status>({ kind: "idle", message: "" });

  useEffect(() => {
    let cancelled = false;
    fetch("/api/settings/model", { cache: "no-store" })
      .then(async (response) => {
        const data = (await response.json()) as Partial<ModelSettings> & {
          error?: string;
        };
        if (!response.ok) {
          throw new Error(data.error ?? `HTTP ${response.status}`);
        }
        if (!cancelled) {
          setValues({
            url: data.url ?? "",
            model: data.model ?? "",
            timeout: data.timeout ?? "",
            configured: Boolean(data.configured),
          });
          setLoading(false);
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setStatus({
            kind: "error",
            message: error instanceof Error ? error.message : "Cannot load the model",
          });
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function update(key: "url" | "model" | "timeout", value: string) {
    setValues((current) => ({ ...current, [key]: value }));
    setStatus({ kind: "idle", message: "" });
  }

  async function connect() {
    setSaving(true);
    setStatus({ kind: "loading", message: "Probing the endpoint..." });
    try {
      const response = await fetch("/api/settings/model", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          url: values.url,
          model: values.model,
          timeout: values.timeout,
          apiKey,
          clearKey: false,
        }),
      });
      const data = (await response.json()) as {
        ok?: boolean;
        configured?: boolean;
        error?: string;
      };
      if (!response.ok || !data.ok) {
        throw new Error(data.error ?? `HTTP ${response.status}`);
      }
      setValues((current) => ({ ...current, configured: Boolean(data.configured) }));
      setApiKey("");
      setStatus({
        kind: "saved",
        message:
          "Connected and saved. The voice client applies the change within 30 seconds.",
      });
      onSaved?.();
    } catch (error) {
      setStatus({
        kind: "error",
        message: error instanceof Error ? error.message : "Cannot connect the model",
      });
    } finally {
      setSaving(false);
    }
  }

  async function clearKey() {
    setSaving(true);
    try {
      const response = await fetch("/api/settings/model", { method: "DELETE" });
      const data = (await response.json()) as { error?: string };
      if (!response.ok) {
        throw new Error(data.error ?? `HTTP ${response.status}`);
      }
      setValues((current) => ({ ...current, configured: false }));
      setStatus({ kind: "saved", message: "API key cleared." });
      onSaved?.();
    } catch (error) {
      setStatus({
        kind: "error",
        message: error instanceof Error ? error.message : "Cannot clear the key",
      });
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return <p className="text-xs text-muted-foreground">Loading the model...</p>;
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="space-y-2 sm:col-span-2">
          <span className="text-sm font-medium">Endpoint URL</span>
          <Input
            value={values.url}
            onChange={(event) => update("url", event.target.value)}
            placeholder="http://127.0.0.1:8080"
            maxLength={500}
            className="text-sm"
          />
          <span className="block text-[10px] text-muted-foreground">
            Base URL of an OpenAI-compatible chat-completions endpoint.
          </span>
        </label>
        <label className="space-y-2">
          <span className="text-sm font-medium">Model name</span>
          <Input
            value={values.model}
            onChange={(event) => update("model", event.target.value)}
            placeholder="local"
            maxLength={200}
            className="text-sm"
          />
        </label>
        <label className="space-y-2">
          <span className="text-sm font-medium">Timeout</span>
          <Input
            value={values.timeout}
            onChange={(event) => update("timeout", event.target.value)}
            placeholder="30s"
            maxLength={50}
            className="text-sm"
          />
        </label>
        <label className="space-y-2 sm:col-span-2">
          <span className="flex items-center gap-2 text-sm font-medium">
            API key
            {values.configured && (
              <Badge variant="outline" className="rounded-none px-1.5 py-0 text-[9px] uppercase tracking-wide">
                configured
              </Badge>
            )}
          </span>
          <Input
            type="password"
            value={apiKey}
            onChange={(event) => setApiKey(event.target.value)}
            placeholder={values.configured ? "Write-only. Type a new key to replace it." : "Optional"}
            maxLength={500}
            className="text-sm"
          />
          <span className="block text-[10px] text-muted-foreground">
            Stored write-only. It is never shown again.
          </span>
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={connect} disabled={saving || values.url.trim() === ""}>
          {saving ? "Connecting..." : "Connect model"}
        </Button>
        {values.configured && (
          <Button variant="outline" onClick={clearKey} disabled={saving}>
            Clear key
          </Button>
        )}
      </div>
      {status.message && (
        <p
          role="status"
          className={
            status.kind === "error" ? "text-xs text-destructive" : "text-xs text-muted-foreground"
          }
        >
          {status.message}
        </p>
      )}
    </div>
  );
}
