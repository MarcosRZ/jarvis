import { NextResponse } from "next/server";
import { readSettings, saveSettings } from "@/lib/settings-server";
import { settingField, validateSettingValue } from "@/lib/settings-schema";

const noStore = { "Cache-Control": "no-store" };
const keys = {
  url: "VOICE_LLM_URL",
  model: "VOICE_LLM_MODEL",
  timeout: "VOICE_LLM_TIMEOUT",
  apiKey: "VOICE_LLM_API_KEY",
} as const;

type ModelBody = {
  url?: unknown;
  model?: unknown;
  timeout?: unknown;
  apiKey?: unknown;
  clearKey?: unknown;
};

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function validate(key: string, value: string): string | null {
  const field = settingField(key);
  if (!field) {
    return `${key} is not a known setting.`;
  }
  return validateSettingValue(field, value);
}

// probeBase translates a loopback address into the host gateway: the client
// runs on the host and uses 127.0.0.1, but this route runs in the dashboard
// container, where loopback is the container itself.
function probeBase(url: string): string {
  try {
    const parsed = new URL(url);
    if (parsed.hostname === "127.0.0.1" || parsed.hostname === "localhost" || parsed.hostname === "::1") {
      parsed.hostname = "host.docker.internal";
    }
    return parsed.toString().replace(/\/+$/, "");
  } catch {
    return url.replace(/\/+$/, "");
  }
}

async function probe(url: string, model: string, apiKey: string): Promise<string | null> {
  const base = probeBase(url);
  const headers: Record<string, string> = { Accept: "application/json" };
  if (apiKey) {
    headers.Authorization = `Bearer ${apiKey}`;
  }
  try {
    const models = await fetch(`${base}/v1/models`, {
      headers,
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
    });
    if (models.ok) {
      return null;
    }
    if (models.status === 401 || models.status === 403) {
      return `The endpoint rejected the API key (HTTP ${models.status}).`;
    }
    if (models.status !== 404 && models.status !== 405) {
      return `The endpoint answered HTTP ${models.status}.`;
    }
  } catch (error) {
    return error instanceof Error ? error.message : "The endpoint is unreachable.";
  }

  // Some OpenAI-compatible servers do not expose /v1/models. A one-token
  // completion is a cheap equivalent probe.
  try {
    const response = await fetch(`${base}/v1/chat/completions`, {
      method: "POST",
      headers: { ...headers, "Content-Type": "application/json" },
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
      body: JSON.stringify({
        model: model || "local",
        messages: [{ role: "user", content: "ping" }],
        max_tokens: 1,
        temperature: 0,
      }),
    });
    if (response.ok) {
      return null;
    }
    const detail = (await response.text()).slice(0, 200).trim();
    return `The endpoint answered HTTP ${response.status}${detail ? `: ${detail}` : ""}.`;
  } catch (error) {
    return error instanceof Error ? error.message : "The endpoint is unreachable.";
  }
}

export async function GET() {
  try {
    const values = await readSettings();
    return NextResponse.json(
      {
        url: values[keys.url] ?? "",
        model: values[keys.model] ?? "",
        timeout: values[keys.timeout] ?? "",
        configured: Boolean(values[keys.apiKey]),
      },
      { headers: noStore },
    );
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Cannot read the settings" },
      { status: 500, headers: noStore },
    );
  }
}

export async function POST(request: Request) {
  let body: ModelBody;
  try {
    body = (await request.json()) as ModelBody;
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400, headers: noStore });
  }
  const url = text(body.url);
  const model = text(body.model);
  const timeout = text(body.timeout);
  const apiKey = text(body.apiKey);
  for (const [key, value] of [
    [keys.url, url],
    [keys.model, model],
    [keys.timeout, timeout],
  ] as const) {
    if (value === "") {
      continue;
    }
    const problem = validate(key, value);
    if (problem) {
      return NextResponse.json({ error: problem }, { status: 400, headers: noStore });
    }
  }
  if (url === "") {
    return NextResponse.json({ error: "The endpoint URL is required." }, { status: 400, headers: noStore });
  }

  const existing = await readSettings();
  const key = apiKey || existing[keys.apiKey] || "";
  const problem = await probe(url, model, key);
  if (problem) {
    return NextResponse.json({ error: problem }, { status: 502, headers: noStore });
  }

  const next = { ...existing };
  next[keys.url] = url;
  if (model !== "") {
    next[keys.model] = model;
  }
  if (timeout !== "") {
    next[keys.timeout] = timeout;
  }
  if (body.clearKey === true) {
    delete next[keys.apiKey];
  } else if (apiKey !== "") {
    next[keys.apiKey] = apiKey;
  }
  try {
    await saveSettings(next);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Cannot save the settings" },
      { status: 500, headers: noStore },
    );
  }
  return NextResponse.json({ ok: true, configured: Boolean(next[keys.apiKey]) }, { headers: noStore });
}

export async function DELETE() {
  try {
    const existing = await readSettings();
    delete existing[keys.apiKey];
    await saveSettings(existing);
    return NextResponse.json({ ok: true, configured: false }, { headers: noStore });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Cannot clear the key" },
      { status: 500, headers: noStore },
    );
  }
}
