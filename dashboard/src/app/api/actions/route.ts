import { NextResponse } from "next/server";

const collectorURL = process.env.COLLECTOR_URL ?? "http://collector:8090";
const timeoutMs = 8000;
const runTimeoutMs = 12000;
const noStore = { "Cache-Control": "no-store" };

export async function GET() {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(`${collectorURL}/v1/actions`, {
      cache: "no-store",
      signal: controller.signal,
    });
    const body = await response.text();
    return new NextResponse(body, {
      status: response.status,
      headers: {
        "Content-Type": "application/json",
        ...noStore,
      },
    });
  } catch {
    const aborted = controller.signal.aborted;
    return NextResponse.json(
      { error: aborted ? "collector_timeout" : "collector_unavailable" },
      {
        status: aborted ? 504 : 502,
        headers: noStore,
      },
    );
  } finally {
    clearTimeout(timer);
  }
}

// POST runs one catalog action. Only the action ID crosses the boundary: the
// collector looks up the fixed service in its own catalog.
export async function POST(request: Request) {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "invalid_json" }, { status: 400, headers: noStore });
  }
  const id =
    typeof body === "object" && body !== null && "id" in body && typeof body.id === "string"
      ? body.id.trim()
      : "";
  if (id === "") {
    return NextResponse.json({ error: "action_required" }, { status: 400, headers: noStore });
  }

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), runTimeoutMs);
  try {
    const response = await fetch(`${collectorURL}/v1/actions/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
      signal: controller.signal,
      body: JSON.stringify({ id }),
    });
    const text = await response.text();
    return new NextResponse(text, {
      status: response.status,
      headers: {
        "Content-Type": "application/json",
        ...noStore,
      },
    });
  } catch {
    const aborted = controller.signal.aborted;
    return NextResponse.json(
      { error: aborted ? "collector_timeout" : "collector_unavailable" },
      {
        status: aborted ? 504 : 502,
        headers: noStore,
      },
    );
  } finally {
    clearTimeout(timer);
  }
}
