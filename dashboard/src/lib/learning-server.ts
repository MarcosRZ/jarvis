import { promises as fs } from "node:fs";
import path from "node:path";
import { normalizeLearningText } from "@/lib/learning";
import type {
  LearningEntry,
  LearningOperation,
  LearningOperationInput,
} from "@/lib/learning";

export const learningFile = "/data/learning.json";
export const operationsFile = "/data/learning-operations.json";
export const snapshotVersion = 1;
export const maxBodyBytes = 2 * 1024 * 1024;
export const maxOperationsBodyBytes = 256 * 1024;
export const maxEntries = 5000;
export const maxOperations = 5000;
export const maxOperationTexts = 500;
export const maxTextLength = 500;
export const maxIdentifierLength = 300;
export const maxNameLength = 300;
export const maxKindLength = 40;

export type StoredSnapshot = {
  version: number;
  generatedAt: string;
  receivedAt: string;
  entries: LearningEntry[];
};

export type OperationsState = {
  nextId: number;
  operations: LearningOperation[];
};

export class DuplicateLearningTextError extends Error {}

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

export function parseEntry(
  value: unknown,
  where: string,
  options: { requireUpdated?: boolean } = {},
): LearningEntry {
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
  if (kind !== undefined && kind !== "turn-on" && kind !== "turn-off") {
    throw new Error(`${where}.kind must be turn-on or turn-off`);
  }
  if (options.requireUpdated !== false && value.updated === undefined) {
    throw new Error(`${where}.updated must be an RFC 3339 timestamp`);
  }

  const entry: LearningEntry = {
    text,
    score: integer(value.score, `${where}.score`),
    successes: nonNegativeInteger(value.successes, `${where}.successes`),
    failures: nonNegativeInteger(value.failures, `${where}.failures`),
    updated:
      value.updated === undefined
        ? new Date(0).toISOString()
        : timestamp(value.updated, `${where}.updated`),
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

export function parseSnapshot(value: unknown): Omit<
  StoredSnapshot,
  "receivedAt"
> & { appliedThrough: number } {
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
    appliedThrough:
      value.appliedThrough === undefined
        ? 0
        : nonNegativeInteger(value.appliedThrough, "appliedThrough"),
    entries: value.entries.map((entry, index) =>
      parseEntry(entry, `entries[${index}]`),
    ),
  };
}

export async function readSnapshot(): Promise<StoredSnapshot | null> {
  try {
    const raw = await fs.readFile(learningFile, "utf8");
    const parsed = JSON.parse(raw) as unknown;
    const snapshot = parseSnapshot(parsed);
    if (!isRecord(parsed)) {
      throw new Error("saved snapshot must be a JSON object");
    }
    return {
      version: snapshot.version,
      generatedAt: snapshot.generatedAt,
      receivedAt: timestamp(parsed.receivedAt, "receivedAt"),
      entries: snapshot.entries,
    };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return null;
    }
    throw error;
  }
}

export async function writeSnapshot(snapshot: StoredSnapshot): Promise<void> {
  await writeJSON(learningFile, snapshot);
}

function parseOperation(value: unknown, index: number): LearningOperation {
  const where = `operations[${index}]`;
  if (!isRecord(value)) {
    throw new Error(`${where} must be an object`);
  }
  const id = nonNegativeInteger(value.id, `${where}.id`);
  if (id === 0) {
    throw new Error(`${where}.id must be greater than zero`);
  }
  const createdAt = timestamp(value.createdAt, `${where}.createdAt`);
  const kind = value.kind;
  if (kind === "upsert") {
    const entry = parseEntry(value.entry, `${where}.entry`);
    const originalText = optionalString(
      value.originalText,
      `${where}.originalText`,
      maxTextLength,
    );
    return {
      id,
      kind,
      createdAt,
      entry,
      ...(originalText ? { originalText } : {}),
    };
  }
  if (kind === "delete") {
    if (!Array.isArray(value.texts) || value.texts.length === 0) {
      throw new Error(`${where}.texts must be a non-empty array`);
    }
    if (value.texts.length > maxOperationTexts) {
      throw new Error(
        `${where}.texts must have at most ${maxOperationTexts} items`,
      );
    }
    const texts = value.texts.map((text, textIndex) =>
      entryText(text, `${where}.texts[${textIndex}]`),
    );
    return { id, kind, createdAt, texts };
  }
  throw new Error(`${where}.kind must be upsert or delete`);
}

function parseOperationInput(value: unknown): LearningOperationInput {
  if (!isRecord(value)) {
    throw new Error("body must be a JSON object");
  }
  if (value.kind === "upsert") {
    const originalText = optionalString(
      value.originalText,
      "originalText",
      maxTextLength,
    );
    const entry = parseEntry(value.entry, "entry", { requireUpdated: false });
    entry.text = normalizeInputText(entry.text, "entry.text");
    return {
      kind: "upsert",
      ...(originalText
        ? { originalText: normalizeInputText(originalText, "originalText") }
        : {}),
      entry,
    };
  }
  if (value.kind === "delete") {
    if (!Array.isArray(value.texts) || value.texts.length === 0) {
      throw new Error("texts must be a non-empty array");
    }
    if (value.texts.length > maxOperationTexts) {
      throw new Error(`texts must have at most ${maxOperationTexts} items`);
    }
    const texts = value.texts.map((text, index) =>
      normalizeInputText(entryText(text, `texts[${index}]`), `texts[${index}]`),
    );
    return { kind: "delete", texts: [...new Set(texts)] };
  }
  throw new Error("kind must be upsert or delete");
}

export async function readOperations(): Promise<OperationsState> {
  try {
    const raw = await fs.readFile(operationsFile, "utf8");
    const parsed = JSON.parse(raw) as unknown;
    if (!isRecord(parsed)) {
      throw new Error("saved operations must be a JSON object");
    }
    const nextId = nonNegativeInteger(parsed.nextId ?? 0, "nextId");
    if (!Array.isArray(parsed.operations)) {
      throw new Error("operations must be an array");
    }
    if (parsed.operations.length > maxOperations) {
      throw new Error(`operations must have at most ${maxOperations} items`);
    }
    const operations = parsed.operations.map(parseOperation);
    let previous = 0;
    for (const operation of operations) {
      if (operation.id <= previous) {
        throw new Error("operations must be sorted by increasing id");
      }
      previous = operation.id;
    }
    return { nextId, operations };
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return { nextId: 0, operations: [] };
    }
    throw error;
  }
}

async function writeOperations(state: OperationsState): Promise<void> {
  await writeJSON(operationsFile, state);
}

async function writeJSON(file: string, value: unknown): Promise<void> {
  const directory = path.dirname(file);
  const temporary = `${file}.${process.pid}.${Date.now()}.tmp`;
  try {
    await fs.mkdir(directory, { recursive: true });
    await fs.writeFile(temporary, JSON.stringify(value, null, 2), {
      mode: 0o600,
    });
    await fs.rename(temporary, file);
  } catch (error) {
    await fs.rm(temporary, { force: true }).catch(() => {});
    throw error;
  }
}

let operationsLock: Promise<unknown> = Promise.resolve();

function withOperationsLock<T>(task: () => Promise<T>): Promise<T> {
  const run = operationsLock.then(task, task);
  operationsLock = run.then(
    () => undefined,
    () => undefined,
  );
  return run;
}

export function applyPendingOperations(
  entries: LearningEntry[],
  operations: LearningOperation[],
): LearningEntry[] {
  const byText = new Map(entries.map((entry) => [entry.text, entry]));
  for (const operation of operations) {
    if (operation.kind === "delete") {
      for (const text of operation.texts ?? []) {
        byText.delete(text);
      }
      continue;
    }
    if (!operation.entry) {
      continue;
    }
    if (
      operation.originalText &&
      operation.originalText !== operation.entry.text
    ) {
      byText.delete(operation.originalText);
    }
    byText.set(operation.entry.text, operation.entry);
  }
  return [...byText.values()].sort((left, right) =>
    left.text.localeCompare(right.text),
  );
}

export function parseOperationInputBody(body: unknown): LearningOperationInput {
  return parseOperationInput(body);
}

function normalizeUpsert(
  input: Extract<LearningOperationInput, { kind: "upsert" }>,
): Extract<LearningOperationInput, { kind: "upsert" }> {
  const text = normalizeInputText(input.entry.text, "entry.text");
  return {
    kind: "upsert",
    ...(input.originalText
      ? { originalText: normalizeInputText(input.originalText, "originalText") }
      : {}),
    entry: { ...input.entry, text },
  };
}

function normalizeInputText(text: string, field: string): string {
  const normalized = normalizeLearningText(text);
  if (normalized === "") {
    throw new Error(`${field} is empty after normalization`);
  }
  return normalized;
}

export async function enqueueOperation(
  input: LearningOperationInput,
): Promise<{ operation: LearningOperation; pendingOperations: number }> {
  const normalized =
    input.kind === "upsert" ? normalizeUpsert(input) : input;
  return withOperationsLock(async () => {
    const [state, snapshot] = await Promise.all([
      readOperations(),
      readSnapshot(),
    ]);
    const effective = applyPendingOperations(
      snapshot?.entries ?? [],
      state.operations,
    );
    if (normalized.kind === "upsert") {
      const exists = effective.some(
        (entry) => entry.text === normalized.entry.text,
      );
      if (exists && normalized.entry.text !== normalized.originalText) {
        throw new DuplicateLearningTextError(
          "That sentence already exists. Change the sentence or edit the existing association.",
        );
      }
    }

    const now = new Date();
    const id = Math.max(state.nextId + 1, now.getTime());
    if (!Number.isSafeInteger(id)) {
      throw new Error("operation id overflow");
    }
    const operation: LearningOperation =
      normalized.kind === "upsert"
        ? {
            id,
            kind: "upsert",
            createdAt: now.toISOString(),
            ...(normalized.originalText
              ? { originalText: normalized.originalText }
              : {}),
            entry: { ...normalized.entry, updated: now.toISOString() },
          }
        : {
            id,
            kind: "delete",
            createdAt: now.toISOString(),
            texts: normalized.texts.map((text) =>
              normalizeInputText(text, "texts"),
            ),
          };
    state.nextId = id;
    state.operations.push(operation);
    if (state.operations.length > maxOperations) {
      throw new Error(
        `the queue already has the maximum of ${maxOperations} operations`,
      );
    }
    await writeOperations(state);
    return {
      operation,
      pendingOperations: state.operations.length,
    };
  });
}

export async function acknowledgeOperations(
  appliedThrough: number,
): Promise<number> {
  if (appliedThrough <= 0) {
    return 0;
  }
  return withOperationsLock(async () => {
    const state = await readOperations();
    const remaining = state.operations.filter(
      (operation) => operation.id > appliedThrough,
    );
    const floor =
      appliedThrough >= Number.MAX_SAFE_INTEGER - 1
        ? Number.MAX_SAFE_INTEGER
        : appliedThrough + 1;
    const nextId = Math.max(state.nextId, floor);
    if (
      remaining.length === state.operations.length &&
      nextId === state.nextId
    ) {
      return remaining.length;
    }
    await writeOperations({ nextId, operations: remaining });
    return remaining.length;
  });
}

export function pendingSince(operations: LearningOperation[]): string | null {
  if (operations.length === 0) {
    return null;
  }
  return operations.reduce(
    (earliest, operation) =>
      operation.createdAt < earliest ? operation.createdAt : earliest,
    operations[0].createdAt,
  );
}
