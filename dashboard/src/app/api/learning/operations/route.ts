import { NextResponse } from "next/server";
import {
  DuplicateLearningTextError,
  enqueueOperation,
  maxOperationsBodyBytes,
  parseOperationInputBody,
  readOperations,
} from "@/lib/learning-server";

export async function GET(request: Request) {
  const noStore = { "Cache-Control": "no-store" };
  const rawAfter = new URL(request.url).searchParams.get("after") ?? "0";
  const after = Number(rawAfter);
  if (!Number.isSafeInteger(after) || after < 0) {
    return NextResponse.json(
      { error: "after must be a non-negative integer" },
      { status: 400, headers: noStore },
    );
  }
  try {
    const state = await readOperations();
    const operations = state.operations.filter(
      (operation) => operation.id > after,
    );
    return NextResponse.json(
      { operations, pendingOperations: state.operations.length },
      { headers: noStore },
    );
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error ? error.message : "Cannot read learning queue",
      },
      { status: 500, headers: noStore },
    );
  }
}

export async function POST(request: Request) {
  const declaredLength = Number(request.headers.get("content-length") ?? "0");
  if (
    Number.isFinite(declaredLength) &&
    declaredLength > maxOperationsBodyBytes
  ) {
    return NextResponse.json(
      { error: `body must be at most ${maxOperationsBodyBytes} bytes` },
      { status: 413 },
    );
  }

  let body: unknown;
  try {
    const raw = await request.text();
    if (Buffer.byteLength(raw, "utf8") > maxOperationsBodyBytes) {
      return NextResponse.json(
        { error: `body must be at most ${maxOperationsBodyBytes} bytes` },
        { status: 413 },
      );
    }
    body = JSON.parse(raw) as unknown;
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  let input: ReturnType<typeof parseOperationInputBody>;
  try {
    input = parseOperationInputBody(body);
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Invalid operation",
      },
      { status: 400 },
    );
  }

  try {
    const result = await enqueueOperation(input);
    return NextResponse.json(result, {
      status: 201,
      headers: { "Cache-Control": "no-store" },
    });
  } catch (error) {
    if (error instanceof DuplicateLearningTextError) {
      return NextResponse.json({ error: error.message }, { status: 409 });
    }
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Cannot queue operation",
      },
      { status: 500 },
    );
  }
}
