import { promises as fs } from "node:fs";
import path from "node:path";
import { NextResponse } from "next/server";

const learningFile = "/data/learning.json";
const snapshotVersion = 1;
const staleAfterMs = 5 * 60 * 1000;
const maxBodyBytes = 2 * 1024 * 1024;
const maxEntries = 5000;
const maxTextLength = 500;
const maxIdentifierLength = 300;
const maxNameLength = 300;
const maxKindLength = 40;

type LearningEntry = {
  text: string;
  device?: string;
  name?: string;
  kind?: string;
  action?: string;
  score: number;
  successes: number;
  failures: number;
  updated: string;
};

type StoredSnapshot = {
  version: number;
  generatedAt: string;
  receivedAt: string;
  entries: LearningEntry[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function entryText(value: unknown, field: string): string {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${field} must be a non-empty string`);
  }
  if (value.length > maxTextLength) {
    throw new Error(`${field} must be at most ${maxTextLength} characters`);
  }
  return value;
}

function optionalString(
  value: unknown,
  field: string,
  maxLength: number,
): string | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  if (typeof value !== "string") {
    throw new Error(`${field} must be a string`);
  }
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  if (trimmed.length > maxLength) {
    throw new Error(`${field} must be at most ${maxLength} characters`);
  }
  return trimmed;
}

function timestamp(value: unknown, field: string): string {
  if (typeof value !== "string") {
    throw new Error(`${field} must be an RFC 3339 timestamp`);
  }
  const milliseconds = Date.parse(value);
  if (Number.isNaN(milliseconds)) {
    throw new Error(`${field} must be an RFC 3339 timestamp`);
  }
  return new Date(milliseconds).toISOString();
}

function integer(value: unknown, field: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value)) {
    throw new Error(`${field} must be an integer`);
  }
  return value;
}

function nonNegativeInteger(value: unknown, field: string): number {
  const parsed = integer(value, field);
  if (parsed < 0) {
    throw new Error(`${field} must be zero or greater`);
  }
  return parsed;
}

function parseEntry(value: unknown, index: number): LearningEntry {
  const where = `entries[${index}]`;
  if (!isRecord(value)) {
    throw new Error(`${where} must be an object`);
  }
  const text = entryText(value.text, `${where}.text`);
  const device = optionalString(
    value.device,
    `${where}.device`,
    maxIdentifierLength,
  );
  const action = optionalString(
    value.action,
    `${where}.action`,
    maxIdentifierLength,
  );
  const name = optionalString(value.name, `${where}.name`, maxNameLength);
  const kind = optionalString(value.kind, `${where}.kind`, maxKindLength);
  if ((device === undefined) === (action === undefined)) {
    throw new Error(`${where} must name exactly one of device or action`);
  }
  if (device !== undefined && kind === undefined) {
    throw new Error(`${where}.kind is required for a device`);
  }
  if (device === undefined && kind !== undefined) {
    throw new Error(`${where}.kind is only valid for a device`);
  }

  const entry: LearningEntry = {
    text,
    score: integer(value.score, `${where}.score`),
    successes: nonNegativeInteger(value.successes, `${where}.successes`),
    failures: nonNegativeInteger(value.failures, `${where}.failures`),
    updated: timestamp(value.updated, `${where}.updated`),
  };
  if (device !== undefined) {
    entry.device = device;
  }
  if (name !== undefined) {
    entry.name = name;
  }
  if (kind !== undefined) {
    entry.kind = kind;
  }
  if (action !== undefined) {
    entry.action = action;
  }
  return entry;
}

function parseSnapshot(value: unknown): Omit<StoredSnapshot, "receivedAt"> {
  if (!isRecord(value)) {
    throw new Error("body must be a JSON object");
  }
  if (value.version !== snapshotVersion) {
    throw new Error(`version must be ${snapshotVersion}`);
  }
  const generatedAt = timestamp(value.generatedAt, "generatedAt");
  if (!Array.isArray(value.entries)) {
    throw new Error("entries must be an array");
  }
  if (value.entries.length > maxEntries) {
    throw new Error(`entries must have at most ${maxEntries} items`);
  }
  return {
    version: snapshotVersion,
    generatedAt,
    entries: value.entries.map(parseEntry),
  };
}

async function readSnapshot(): Promise<StoredSnapshot | null> {
  try {
    const raw = await fs.readFile(learningFile, "utf8");
    const parsed = JSON.parse(raw) as unknown;
    const snapshot = parseSnapshot(parsed);
    if (!isRecord(parsed)) {
      throw new Error("saved snapshot must be a JSON object");
    }
    return {
      ...snapshot,
      receivedAt: timestamp(parsed.receivedAt, "receivedAt"),
    };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return null;
    }
    throw error;
  }
}

export async function GET() {
  const noStore = { "Cache-Control": "no-store" };
  try {
    const snapshot = await readSnapshot();
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
          entries: [],
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

  let snapshot: Omit<StoredSnapshot, "receivedAt">;
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
    ...snapshot,
    receivedAt: new Date().toISOString(),
  };
  const directory = path.dirname(learningFile);
  const temporary = `${learningFile}.${process.pid}.${Date.now()}.tmp`;
  try {
    await fs.mkdir(directory, { recursive: true });
    await fs.writeFile(temporary, JSON.stringify(stored, null, 2), {
      mode: 0o600,
    });
    await fs.rename(temporary, learningFile);
    return NextResponse.json(
      {
        version: stored.version,
        generatedAt: stored.generatedAt,
        receivedAt: stored.receivedAt,
        entries: stored.entries.length,
      },
      { headers: { "Cache-Control": "no-store" } },
    );
  } catch (error) {
    await fs.rm(temporary, { force: true }).catch(() => {});
    return NextResponse.json(
      {
        error:
          error instanceof Error ? error.message : "Cannot save learning snapshot",
      },
      { status: 500 },
    );
  }
}
