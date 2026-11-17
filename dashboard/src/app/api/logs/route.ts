import { NextResponse } from "next/server";

const collectorURL = process.env.COLLECTOR_URL ?? "http://collector:8090";
const timeoutMs = 8000;

export async function GET(request: Request) {
  const incoming = new URL(request.url).searchParams;
  const outgoing = new URLSearchParams();
  for (const key of ["cursor", "limit", "service", "level"]) {
    const value = incoming.get(key);
    if (value !== null) {
      outgoing.set(key, value);
    }
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(`${collectorURL}/v1/logs?${outgoing.toString()}`, {
      cache: "no-store",
      signal: controller.signal,
    });
    const body = await response.text();
    return new NextResponse(body, {
      status: response.status,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-store",
      },
    });
  } catch {
    const aborted = controller.signal.aborted;
    return NextResponse.json(
      { error: aborted ? "collector_timeout" : "collector_unavailable" },
      {
        status: aborted ? 504 : 502,
        headers: { "Cache-Control": "no-store" },
      },
    );
  } finally {
    clearTimeout(timer);
  }
}
