"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { DEFAULT_PROMPT } from "@/lib/default-prompt";

const maxPromptLength = 20000;

type Status = {
  kind: "idle" | "loading" | "saving" | "saved" | "error";
  message: string;
};

export function PromptSettings() {
  const [prompt, setPrompt] = useState(DEFAULT_PROMPT);
  const [savedPrompt, setSavedPrompt] = useState(DEFAULT_PROMPT);
  const [storedPrompt, setStoredPrompt] = useState("");
  const [status, setStatus] = useState<Status>({
    kind: "loading",
    message: "Loading prompt...",
  });

  useEffect(() => {
    let cancelled = false;
    fetch("/api/settings/prompt", { cache: "no-store" })
      .then(async (response) => {
        const data = (await response.json()) as { prompt?: string; error?: string };
        if (!response.ok) {
          throw new Error(data.error ?? `HTTP ${response.status}`);
        }
        if (!cancelled) {
          const stored = data.prompt ?? "";
          setStoredPrompt(stored);
          setPrompt(stored || DEFAULT_PROMPT);
          setSavedPrompt(stored || DEFAULT_PROMPT);
          setStatus({ kind: "idle", message: "" });
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setStatus({
            kind: "error",
            message: error instanceof Error ? error.message : "Cannot load prompt",
          });
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function save(value: string) {
    setStatus({ kind: "saving", message: "Saving..." });
    try {
      const response = await fetch("/api/settings/prompt", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: value }),
      });
      const data = (await response.json()) as { prompt?: string; error?: string };
      if (!response.ok) {
        throw new Error(data.error ?? `HTTP ${response.status}`);
      }
      const stored = data.prompt ?? value;
      setStoredPrompt(stored);
      setPrompt(stored || DEFAULT_PROMPT);
      setSavedPrompt(stored || DEFAULT_PROMPT);
      setStatus({
        kind: "saved",
        message: value.trim() === "" ? "Built-in prompt restored." : "Prompt saved.",
      });
    } catch (error) {
      setStatus({
        kind: "error",
        message: error instanceof Error ? error.message : "Cannot save prompt",
      });
    }
  }

  const dirty = prompt !== savedPrompt;
  const busy = status.kind === "loading" || status.kind === "saving";

  return (
    <Card>
      <CardHeader>
        <CardTitle>Language model prompt</CardTitle>
        <CardDescription>
          The editor shows the built-in prompt when no custom prompt is saved. Edit
          it and save to create a custom prompt. The Home Assistant context and the
          JSON reply format are added automatically. Use{" "}
          <code className="bg-muted px-1 py-0.5">{"{{context}}"}</code> where the
          read-only context should go; without it, the context is appended.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Textarea
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          maxLength={maxPromptLength}
          rows={16}
          spellCheck={false}
          disabled={busy}
          className="min-h-80 font-mono text-xs leading-relaxed"
          aria-label="Language model prompt"
        />
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>
            {storedPrompt === "" ? "Built-in prompt" : "Custom prompt"} · {prompt.length} /{" "}
            {maxPromptLength} characters
          </span>
          <span>Changes apply the next time the assistant starts.</span>
        </div>
        {status.message && (
          <p
            className={
              status.kind === "error"
                ? "text-sm text-destructive"
                : "text-sm text-muted-foreground"
            }
            role="status"
          >
            {status.message}
          </p>
        )}
      </CardContent>
      <CardFooter className="flex flex-wrap gap-2">
        <Button onClick={() => save(prompt)} disabled={busy || !dirty}>
          {status.kind === "saving" ? "Saving..." : "Save prompt"}
        </Button>
        <Button
          variant="outline"
          onClick={() => save("")}
          disabled={busy || storedPrompt === ""}
        >
          Reset to built-in
        </Button>
      </CardFooter>
    </Card>
  );
}
