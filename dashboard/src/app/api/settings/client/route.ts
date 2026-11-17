import { NextResponse } from "next/server";
import { clientValues, readSettings } from "@/lib/settings-server";

const noStore = { "Cache-Control": "no-store" };

export async function GET() {
  try {
    return NextResponse.json(
      { values: clientValues(await readSettings()) },
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
