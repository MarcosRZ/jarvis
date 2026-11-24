import { NextResponse } from "next/server";

const noStore = { "Cache-Control": "no-store" };
const newsURL = (process.env.NEWS_URL ?? "http://127.0.0.1:8091").replace(/\/+$/, "");

async function readJSON(path: string): Promise<unknown> {
  const response = await fetch(`${newsURL}${path}`, {
    cache: "no-store",
    signal: AbortSignal.timeout(8000),
  });
  if (!response.ok) {
    throw new Error(`The news service answered HTTP ${response.status}.`);
  }
  return response.json();
}

export async function GET() {
  try {
    const [status, items] = await Promise.all([
      readJSON("/status"),
      readJSON("/items?limit=200"),
    ]);
    return NextResponse.json({ status, ...(items as object) }, { headers: noStore });
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error
            ? error.message
            : "The news service is unreachable.",
      },
      { status: 502, headers: noStore },
    );
  }
}

export async function POST(request: Request) {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400, headers: noStore });
  }
  try {
    const response = await fetch(`${newsURL}/config`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
      body: JSON.stringify(body),
    });
    const data = (await response.json()) as unknown;
    if (!response.ok) {
      return NextResponse.json(data, { status: response.status, headers: noStore });
    }
    return NextResponse.json(data, { headers: noStore });
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error
            ? error.message
            : "The news service is unreachable.",
      },
      { status: 502, headers: noStore },
    );
  }
}
