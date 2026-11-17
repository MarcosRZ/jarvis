import { NextResponse } from "next/server";
import {
  acknowledgeOperations,
  applyPendingOperations,
  maxBodyBytes,
  parseSnapshot,
  pendingSince,
  readOperations,
  readSnapshot,
  snapshotVersion,
  writeSnapshot,
  type StoredSnapshot,
} from "@/lib/learning-server";

const staleAfterMs = 5 * 60 * 1000;

export async function GET() {
  const noStore = { "Cache-Control": "no-store" };
  try {
    const [snapshot, operationState] = await Promise.all([
      readSnapshot(),
      readOperations(),
    ]);
    const operations = operationState.operations;
    const entries = applyPendingOperations(
      snapshot?.entries ?? [],
      operations,
    );
    const pendingOperations = operations.length;
    const pendingFrom = pendingSince(operations);
    if (!snapshot) {
      return NextResponse.json(
        {
          version: snapshotVersion,
          generatedAt: null,
          receivedAt: null,
          source: {
            state: "unavailable",
            lastSuccessAt: null,
            message: "No learning snapshot has been published yet.",
          },
          entries,
          pendingOperations,
          pendingSince: pendingFrom,
        },
        { headers: noStore },
      );
    }
    const receivedAt = Date.parse(snapshot.receivedAt);
    const stale =
      !Number.isFinite(receivedAt) || Date.now() - receivedAt > staleAfterMs;
    return NextResponse.json(
      {
        ...snapshot,
        entries,
        pendingOperations,
        pendingSince: pendingFrom,
        source: {
          state: stale ? "degraded" : "ok",
          lastSuccessAt: snapshot.receivedAt,
          message: stale
            ? `The voice client has not published a learning snapshot in the last ${
                staleAfterMs / 60_000
              } minutes. It may be stopped or publishing may be disabled.`
            : null,
        },
      },
      { headers: noStore },
    );
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error ? error.message : "Cannot read learning snapshot",
      },
      { status: 500, headers: noStore },
    );
  }
}

export async function POST(request: Request) {
  const declaredLength = Number(request.headers.get("content-length") ?? "0");
  if (Number.isFinite(declaredLength) && declaredLength > maxBodyBytes) {
    return NextResponse.json(
      { error: `body must be at most ${maxBodyBytes} bytes` },
      { status: 413 },
    );
  }

  let snapshot: ReturnType<typeof parseSnapshot>;
  try {
    const raw = await request.text();
    if (Buffer.byteLength(raw, "utf8") > maxBodyBytes) {
      return NextResponse.json(
        { error: `body must be at most ${maxBodyBytes} bytes` },
        { status: 413 },
      );
    }
    snapshot = parseSnapshot(JSON.parse(raw) as unknown);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Invalid JSON body";
    const status = message.startsWith("body must be at most") ? 413 : 400;
    return NextResponse.json({ error: message }, { status });
  }

  const stored: StoredSnapshot = {
    version: snapshot.version,
    generatedAt: snapshot.generatedAt,
    receivedAt: new Date().toISOString(),
    entries: snapshot.entries,
  };
  try {
    await writeSnapshot(stored);
    const pendingOperations = await acknowledgeOperations(
      snapshot.appliedThrough,
    );
    return NextResponse.json(
      {
        version: stored.version,
        generatedAt: stored.generatedAt,
        receivedAt: stored.receivedAt,
        appliedThrough: snapshot.appliedThrough,
        entries: stored.entries.length,
        pendingOperations,
      },
      { headers: { "Cache-Control": "no-store" } },
    );
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error ? error.message : "Cannot save learning snapshot",
      },
      { status: 500 },
    );
  }
}
