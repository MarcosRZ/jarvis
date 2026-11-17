import { NextResponse } from "next/server";
import {
  InvalidSettingsError,
  parseSettingsBody,
  readSettings,
  saveSettings,
} from "@/lib/settings-server";

const noStore = { "Cache-Control": "no-store" };

export async function GET() {
  try {
    return NextResponse.json(
      { values: await readSettings() },
      { headers: noStore },
    );
  } catch (error) {
    return NextResponse.json(
      {
        error:
          error instanceof Error ? error.message : "Cannot read the settings",
      },
      { status: 500, headers: noStore },
    );
  }
}

export async function POST(request: Request) {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid JSON body" },
      { status: 400, headers: noStore },
    );
  }
  try {
    const values = await saveSettings(parseSettingsBody(body));
    return NextResponse.json({ values }, { headers: noStore });
  } catch (error) {
    const status = error instanceof InvalidSettingsError ? 400 : 500;
    return NextResponse.json(
      {
        error:
          error instanceof Error ? error.message : "Cannot save the settings",
      },
      { status, headers: noStore },
    );
  }
}
