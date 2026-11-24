"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ExternalLink, Loader2, Plus, RefreshCw, Save, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/dashboard/page-header";

type NewsItem = {
  source: string;
  title: string;
  link: string;
  published: string;
};

type FeedStatus = {
  name: string;
  url: string;
  items: number;
  error?: string;
};

type NewsConfig = {
  feeds: { name: string; url: string }[];
  refreshSeconds: number;
  retention: { items: number; days: number; latestOnly: boolean };
};

type NewsStatus = {
  refreshedAt: string;
  refreshing: boolean;
  feeds: FeedStatus[];
  config: NewsConfig;
};

type Status = {
  kind: "idle" | "saved" | "error";
  message: string;
};

const presets = [
  { id: "5", label: "5 items per feed", retention: { items: 5, days: 0, latestOnly: false } },
  { id: "20-7", label: "20 items / 7 days", retention: { items: 20, days: 7, latestOnly: false } },
  { id: "50-30", label: "50 items / 30 days", retention: { items: 50, days: 30, latestOnly: false } },
  { id: "latest", label: "Only the last fetch", retention: { items: 0, days: 0, latestOnly: true } },
];

function presetOf(config: NewsConfig): string {
  if (config.retention.latestOnly) {
    return "latest";
  }
  if (config.retention.items === 5) {
    return "5";
  }
  if (config.retention.items === 50) {
    return "50-30";
  }
  return "20-7";
}

function relativeTime(value: string): string {
  if (!value || Date.parse(value) === 0) {
    return "Never";
  }
  const minutes = Math.round((Date.now() - Date.parse(value)) / 60_000);
  if (!Number.isFinite(minutes)) {
    return "Never";
  }
  if (minutes < 1) {
    return "Just now";
  }
  if (minutes < 60) {
    return `${minutes} min ago`;
  }
  const hours = Math.round(minutes / 60);
  if (hours < 24) {
    return `${hours} h ago`;
  }
  return `${Math.round(hours / 24)} d ago`;
}

export default function NewsPage() {
  const [items, setItems] = useState<NewsItem[]>([]);
  const [status, setStatus] = useState<NewsStatus | null>(null);
  const [config, setConfig] = useState<NewsConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<Status>({ kind: "idle", message: "" });

  const [reload, setReload] = useState(0);

  function requestReload() {
    setLoading(true);
    setReload((current) => current + 1);
  }

  useEffect(() => {
    let cancelled = false;
    async function fetchNews() {
      try {
        const response = await fetch("/api/news", { cache: "no-store" });
        const data = (await response.json()) as {
          items?: NewsItem[];
          status?: NewsStatus;
          error?: string;
        };
        if (!response.ok) {
          throw new Error(data.error ?? `HTTP ${response.status}`);
        }
        if (!cancelled) {
          setItems(data.items ?? []);
          setStatus(data.status ?? null);
          setConfig(data.status?.config ?? null);
          setMessage({ kind: "idle", message: "" });
        }
      } catch (error) {
        if (!cancelled) {
          setMessage({
            kind: "error",
            message: error instanceof Error ? error.message : "Cannot load the news",
          });
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }
    void fetchNews();
    return () => {
      cancelled = true;
    };
  }, [reload]);

  function updateFeed(index: number, field: "name" | "url", value: string) {
    setConfig((current) => {
      if (!current) {
        return current;
      }
      const feeds = current.feeds.map((feed, position) =>
        position === index ? { ...feed, [field]: value } : feed,
      );
      return { ...current, feeds };
    });
  }

  function addFeed() {
    setConfig((current) =>
      current ? { ...current, feeds: [...current.feeds, { name: "", url: "" }] } : current,
    );
  }

  function removeFeed(index: number) {
    setConfig((current) =>
      current
        ? { ...current, feeds: current.feeds.filter((_, position) => position !== index) }
        : current,
    );
  }

  async function save() {
    if (!config) {
      return;
    }
    setSaving(true);
    try {
      const response = await fetch("/api/news", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });
      const data = (await response.json()) as { error?: string };
      if (!response.ok) {
        throw new Error(data.error ?? `HTTP ${response.status}`);
      }
      setMessage({ kind: "saved", message: "Saved. The feeds refresh now." });
      requestReload();
    } catch (error) {
      setMessage({
        kind: "error",
        message: error instanceof Error ? error.message : "Cannot save the news configuration",
      });
    } finally {
      setSaving(false);
    }
  }

  const grouped = config?.feeds.map((feed) => ({
    feed,
    entries: items.filter((item) => item.source === feed.name),
  }));

  return (
    <div className="space-y-6">
      <PageHeader
        title="News"
        description="Headlines from the RSS and Atom feeds read by the voice assistant. Items are kept in memory only."
      >
        <Button variant="outline" onClick={requestReload} disabled={loading}>
          <RefreshCw className={loading ? "size-4 animate-spin" : "size-4"} />
          Refresh
        </Button>
      </PageHeader>

      {message.message && (
        <p
          role="status"
          className={
            message.kind === "error" ? "text-sm text-destructive" : "text-sm text-muted-foreground"
          }
        >
          {message.message}
        </p>
      )}

      <Card className="gap-0 py-0">
        <CardHeader className="border-b p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold">Feeds and retention</h2>
              <p className="text-xs text-muted-foreground">
                {status
                  ? `Last refresh ${relativeTime(status.refreshedAt)}. ${status.feeds.reduce((total, feed) => total + feed.items, 0)} items in memory.`
                  : "The news service has not answered yet."}
              </p>
            </div>
            <Button onClick={() => void save()} disabled={saving || !config}>
              {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
              Save
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 p-5">
          {loading && !config ? (
            <Skeleton className="h-40 w-full" />
          ) : config ? (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <label className="space-y-2">
                  <span className="text-sm font-medium">Refresh interval (seconds)</span>
                  <Input
                    type="number"
                    min={60}
                    max={86400}
                    value={config.refreshSeconds}
                    onChange={(event) =>
                      setConfig({ ...config, refreshSeconds: Number(event.target.value) })
                    }
                    className="text-sm"
                  />
                </label>
                <label className="space-y-2">
                  <span className="text-sm font-medium">Retention</span>
                  <select
                    className="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                    value={presetOf(config)}
                    onChange={(event) => {
                      const preset = presets.find((item) => item.id === event.target.value);
                      if (preset) {
                        setConfig({ ...config, retention: preset.retention });
                      }
                    }}
                  >
                    {presets.map((preset) => (
                      <option key={preset.id} value={preset.id}>
                        {preset.label}
                      </option>
                    ))}
                  </select>
                </label>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-sm font-medium">Feeds</span>
                  <Button variant="outline" size="sm" onClick={addFeed}>
                    <Plus className="size-3.5" />
                    Add feed
                  </Button>
                </div>
                {config.feeds.map((feed, index) => {
                  const feedStatus = status?.feeds.find((item) => item.name === feed.name);
                  return (
                    <div key={index} className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto]">
                      <div className="space-y-1">
                        <Input
                          value={feed.name}
                          onChange={(event) => updateFeed(index, "name", event.target.value)}
                          placeholder="Source name"
                          maxLength={100}
                          className="text-sm"
                        />
                        {feedStatus?.error && (
                          <p className="text-[10px] text-destructive">{feedStatus.error}</p>
                        )}
                      </div>
                      <Input
                        value={feed.url}
                        onChange={(event) => updateFeed(index, "url", event.target.value)}
                        placeholder="https://example.com/feed/"
                        maxLength={1000}
                        className="text-sm"
                      />
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => removeFeed(index)}
                        aria-label={`Remove ${feed.name || "feed"}`}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  );
                })}
              </div>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">
              Start the news service (<code className="font-mono">make services-up</code>) to edit
              the feeds.
            </p>
          )}
        </CardContent>
      </Card>

      {loading && items.length === 0 ? (
        <Skeleton className="h-60 w-full" />
      ) : grouped && grouped.length > 0 ? (
        <div className="grid gap-4 lg:grid-cols-2">
          {grouped.map(({ feed, entries }) => (
            <Card key={feed.name} className="gap-0 py-0">
              <CardHeader className="flex flex-row items-center justify-between border-b p-4">
                <h3 className="text-sm font-semibold">{feed.name}</h3>
                <Badge variant="outline" className="rounded-none text-[10px]">
                  {entries.length}
                </Badge>
              </CardHeader>
              <CardContent className="p-0">
                {entries.length === 0 ? (
                  <p className="p-4 text-xs text-muted-foreground">No recent items.</p>
                ) : (
                  <ul className="divide-y">
                    {entries.map((item) => (
                      <li key={item.link || item.title} className="p-4">
                        <Link
                          href={item.link}
                          target="_blank"
                          rel="noreferrer"
                          className="group flex items-start justify-between gap-3 text-sm hover:text-primary"
                        >
                          <span>{item.title}</span>
                          <ExternalLink className="mt-0.5 size-3.5 shrink-0 opacity-50 group-hover:opacity-100" />
                        </Link>
                        <p className="mt-1 text-[10px] text-muted-foreground">
                          {relativeTime(item.published)}
                        </p>
                      </li>
                    ))}
                  </ul>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      ) : (
        <Card className="p-6 text-sm text-muted-foreground">
          No news items yet. The service reads the feeds at startup and every configured interval.
        </Card>
      )}
    </div>
  );
}
