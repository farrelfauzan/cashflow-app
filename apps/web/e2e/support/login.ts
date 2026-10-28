import { existsSync } from "node:fs";

import type { APIRequestContext, APIResponse } from "@playwright/test";

import type {
  ErrorResponse,
  LoginCodeRequest,
  LoginCodeVerify,
  MeResponse,
} from "../../src/lib/api/generated/model";

import {
  devlogFilePath,
  devlogOffset,
  toE164,
  waitForLoginCode,
} from "./devlog";
import type { SeededStaff } from "./staff";

const SESSION_COOKIE = "cf_session";

export type SignInResult = "signed-in" | "reused";

type SignInOptions = {
  // The setup test's request fixture: same baseURL as the pages, its own cookie jar.
  request: APIRequestContext;
  // Opens a second context to check a saved session (the `playwright` fixture's `request`).
  newRequestContext: (storageState: string) => Promise<APIRequestContext>;
  baseURL: string;
  staff: SeededStaff;
};

// The API's error code, never the request body: the verify body holds the login code.
async function errorCode(response: APIResponse): Promise<string> {
  try {
    const body = (await response.json()) as ErrorResponse;
    return body.error.code;
  } catch {
    return "no JSON error body";
  }
}

async function requestFailure(
  step: string,
  response: APIResponse,
): Promise<Error> {
  const code = await errorCode(response);
  const hints: Record<string, string> = {
    bad_origin:
      " The app's origin is missing from the API's ALLOWED_ORIGINS; add it.",
    rate_limited:
      " The API allows 3 code requests per phone per 15 minutes. Wait, or flush the e2e Redis.",
  };
  return new Error(
    `${step} answered ${response.status()} (${code}).${hints[code] ?? " Is the Go API running at API_DEV_ORIGIN, migrated and seeded with seed-demo?"}`,
  );
}

// A session saved by an earlier run is reused while the API still accepts it, so local reruns
// do not spend the phone's 3 code requests per 15 minutes. A reset database, another API or
// another user fails the check and signs in again.
async function savedSessionIsValid({
  newRequestContext,
  staff,
}: SignInOptions): Promise<boolean> {
  if (!existsSync(staff.storageState)) return false;
  const context = await newRequestContext(staff.storageState);
  try {
    const response = await context.get("/api/me");
    if (!response.ok()) return false;
    const me = (await response.json()) as MeResponse;
    return me.user.phone === toE164(staff.phone);
  } finally {
    await context.dispose();
  }
}

// Signs a seeded staff member in through the real API, the way the login screen will: request
// a code, read it from the devlog file, verify it. The API sets the HttpOnly cf_session cookie
// on the response, and the request context's state (that cookie) is saved for the specs.
// Requests go through the app's /api rewrite with the app's Origin, like a browser's, so the
// API's origin check and ALLOWED_ORIGINS are exercised too. Never logs the code or the cookie.
export async function signIn(options: SignInOptions): Promise<SignInResult> {
  const { request, baseURL, staff } = options;
  if (await savedSessionIsValid(options)) return "reused";

  const devlogFile = devlogFilePath();
  const headers = { Origin: new URL(baseURL).origin };
  const offset = await devlogOffset(devlogFile);

  const codeRequest: LoginCodeRequest = { phone: staff.phone };
  const requested = await request.post("/api/auth/otp/request", {
    headers,
    data: codeRequest,
  });
  if (requested.status() !== 202) {
    throw await requestFailure("Login code request", requested);
  }

  const code = await waitForLoginCode(devlogFile, offset, staff.phone);
  const verify: LoginCodeVerify = { phone: staff.phone, code };
  const verified = await request.post("/api/auth/otp/verify", {
    headers,
    data: verify,
  });
  if (!verified.ok()) {
    throw await requestFailure("Login code verify", verified);
  }
  const me = (await verified.json()) as MeResponse;
  if (me.user.phone !== toE164(staff.phone)) {
    throw new Error(`Signed in as ${me.user.phone}, expected ${staff.phone}.`);
  }

  const state = await request.storageState();
  if (!state.cookies.some((cookie) => cookie.name === SESSION_COOKIE)) {
    throw new Error(`The verify response set no ${SESSION_COOKIE} cookie.`);
  }
  await request.storageState({ path: staff.storageState });
  return "signed-in";
}
