import { NextResponse } from "next/server";

const collectorURL = process.env.COLLECTOR_URL ?? "http://collector:8090";
// The collector waits for Docker to stop and start the container, bounded at
// 45 seconds, so the proxy timeout leaves a small margin.
const timeoutMs = 50_000;
const serviceIDPattern = /^[a-z0-9-]{1,64}$/;

type RestartPayload = {
  id?: unknown;
};

export async function POST(request: Request) {
  let body: RestartPayload;
  try {
    body = (await request.json()) as RestartPayload;
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }
  if (typeof body.id !== "string" || !serviceIDPattern.test(body.id)) {
    return NextResponse.json({ error: "Invalid service id" }, { status: 400 });
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(
      `${collectorURL}/v1/services/${encodeURIComponent(body.id)}/restart`,
      {
        method: "POST",
        cache: "no-store",
        signal: controller.signal,
      },
    );
    const text = await response.text();
    return new NextResponse(text || JSON.stringify({ service: body.id }), {
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
