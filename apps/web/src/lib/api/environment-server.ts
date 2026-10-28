import "server-only";

import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";

import {
  isLoginPath,
  isTenantPagePath,
  loginPathWithNext,
  safeNextPath,
} from "@/lib/auth/next-path";
import { REQUEST_PATH_HEADER, SESSION_COOKIE } from "@/lib/auth/session";

import { apiInternalUrl } from "./api-internal-url";
import type { ApiEnvironment, RequestEnvironment } from "./environment";

// Server components: call the API on the internal network and send the user's session cookie,
// and nothing else from the incoming request.
async function getRequestEnvironment(): Promise<RequestEnvironment> {
  const session = (await cookies()).get(SESSION_COOKIE)?.value;
  return {
    origin: apiInternalUrl(),
    credentials: "omit",
    headers: session ? { Cookie: `${SESSION_COOKIE}=${session}` } : {},
  };
}

// The session is missing, expired or revoked: back to login, then to this page. redirect()
// throws, so rendering stops here. Without a known path (a route src/proxy.ts does not cover)
// or on the login page itself there is no redirect, which avoids a loop; the caller gets the
// ApiError instead. Tenant pages (outside the matcher anyway) never redirect.
async function handleUnauthorized(): Promise<void> {
  const path = safeNextPath((await headers()).get(REQUEST_PATH_HEADER));
  if (path === null || isLoginPath(path) || isTenantPagePath(path)) return;
  redirect(loginPathWithNext(path));
}

export const apiEnvironment: ApiEnvironment = {
  getRequestEnvironment,
  handleUnauthorized,
};
