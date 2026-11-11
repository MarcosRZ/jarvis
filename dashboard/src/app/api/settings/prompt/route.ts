import { promises as fs } from "node:fs";
import path from "node:path";
import { NextResponse } from "next/server";

const promptFile = "/data/prompt.json";
const maxPromptLength = 20000;

type PromptPayload = {
  prompt?: unknown;
};

async function readPrompt(): Promise<string> {
  try {
    const raw = await fs.readFile(promptFile, "utf8");
    const data = JSON.parse(raw) as PromptPayload;
    return typeof data.prompt === "string" ? data.prompt : "";
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return "";
    }
    throw error;
  }
}

export async function GET() {
  try {
    return NextResponse.json({ prompt: await readPrompt() });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Cannot read prompt" },
      { status: 500 },
    );
  }
}

export async function POST(request: Request) {
  let body: PromptPayload;
  try {
    body = (await request.json()) as PromptPayload;
  } catch {
    return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
  }

  if (typeof body.prompt !== "string") {
    return NextResponse.json({ error: "prompt must be a string" }, { status: 400 });
  }
  if (body.prompt.length > maxPromptLength) {
    return NextResponse.json(
      { error: `prompt must be at most ${maxPromptLength} characters` },
      { status: 413 },
    );
  }

  const directory = path.dirname(promptFile);
  const temporary = `${promptFile}.${process.pid}.tmp`;
  try {
    await fs.mkdir(directory, { recursive: true });
    await fs.writeFile(temporary, JSON.stringify({ prompt: body.prompt }, null, 2), {
      mode: 0o600,
    });
    await fs.rename(temporary, promptFile);
    return NextResponse.json({ prompt: body.prompt });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Cannot save prompt" },
      { status: 500 },
    );
  }
}
