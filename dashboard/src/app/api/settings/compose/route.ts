import { NextResponse } from "next/server";
import { composeScript, readSettings } from "@/lib/settings-server";

const headers = {
  "Content-Type": "text/plain; charset=utf-8",
  "Cache-Control": "no-store",
};

export async function GET() {
  try {
    return new NextResponse(composeScript(await readSettings()), {
      headers,
    });
  } catch (error) {
    return new NextResponse(
      `# Cannot read the settings: ${
        error instanceof Error ? error.message : "unknown error"
      }\n`,
      { status: 500, headers },
    );
  }
}
