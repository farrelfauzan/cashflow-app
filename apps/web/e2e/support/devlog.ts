import { open, stat } from "node:fs/promises";

// Reads login codes from the file the Go API's devlog provider appends to (DEVLOG_FILE, see
// "Login codes for end-to-end tests" in services/api/README.md). One JSON line per message:
// time, to (E.164), template, params (in the template's order) and text.

const LOGIN_CODE_TEMPLATE = "login_code";
const POLL_INTERVAL_MS = 100;

type DevlogLine = {
  to: string;
  template: string;
  params: string[];
};

// The path both the API and Playwright use. Checked before a code is requested, so a missing
// setting does not spend one of the phone's few code requests.
export function devlogFilePath(): string {
  const path = process.env.DEVLOG_FILE?.trim();
  if (!path) {
    throw new Error(
      "DEVLOG_FILE is not set. Point it at the file the Go API writes with WHATSAPP_PROVIDER=devlog and the same DEVLOG_FILE.",
    );
  }
  return path;
}

// Local seeded numbers ("0812-0009-0001") as the API stores them ("+6281200090001"). Only the
// shapes the seed uses; anything else is a mistake in the spec.
export function toE164(phone: string): string {
  const digits = phone.replace(/[\s\-.()]/g, "");
  if (/^\+62\d{9,12}$/.test(digits)) return digits;
  if (/^62\d{9,12}$/.test(digits)) return `+${digits}`;
  if (/^0\d{9,12}$/.test(digits)) return `+62${digits.slice(1)}`;
  throw new Error(`Not an Indonesian phone number: ${phone}`);
}

// The current end of the file, taken before a code is requested, so an older code (from an
// earlier run or request) is never picked up. A file that does not exist yet ends at 0.
export async function devlogOffset(path: string): Promise<number> {
  try {
    return (await stat(path)).size;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return 0;
    throw error;
  }
}

async function readFrom(path: string, offset: number): Promise<string> {
  let file;
  try {
    file = await open(path, "r");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return "";
    throw error;
  }
  try {
    const { size } = await file.stat();
    if (size <= offset) return "";
    const buffer = Buffer.alloc(size - offset);
    await file.read(buffer, 0, buffer.length, offset);
    return buffer.toString("utf8");
  } finally {
    await file.close();
  }
}

function lastLoginCode(text: string, to: string): string | undefined {
  let code: string | undefined;
  // Only complete lines; the API writes each line in one write, but a reader can still land
  // between two of them.
  for (const raw of text.split("\n").slice(0, -1)) {
    if (raw.trim() === "") continue;
    const line = JSON.parse(raw) as DevlogLine;
    if (line.to === to && line.template === LOGIN_CODE_TEMPLATE) {
      code = line.params[0];
    }
  }
  return code;
}

// The last login code for the phone written after `offset`. The API writes it before it
// answers the request, so this normally succeeds on the first read; the poll covers a slow
// disk. Never log the returned code.
export async function waitForLoginCode(
  path: string,
  offset: number,
  phone: string,
  timeoutMs = 5_000,
): Promise<string> {
  const to = toE164(phone);
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const code = lastLoginCode(await readFrom(path, offset), to);
    if (code !== undefined) return code;
    if (Date.now() >= deadline) {
      throw new Error(
        `No login code for ${to} in DEVLOG_FILE after ${timeoutMs}ms. Is the API running with WHATSAPP_PROVIDER=devlog and this same DEVLOG_FILE, and is the phone seeded (seed-demo)?`,
      );
    }
    await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
  }
}
