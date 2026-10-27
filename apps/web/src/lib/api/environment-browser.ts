import {
  isLoginPath,
  loginPathWithNext,
  safeNextPath,
} from "@/lib/auth/next-path";
import { clearQueryCache } from "@/lib/query/query-client";

import type { ApiEnvironment, RequestEnvironment } from "./environment";

// The browser: same-origin /api, so the HttpOnly cf_session cookie goes along by itself. No
// token in JavaScript, no Authorization header, nothing stored.
async function getRequestEnvironment(): Promise<RequestEnvironment> {
  if (typeof window === "undefined") {
    // A client component rendered on the server. TanStack Query does not fetch there; a
    // server-side read belongs in a server component, which uses environment-server.ts.
    throw new Error(
      "The browser API client cannot run during server rendering. Read the data in a server component instead.",
    );
  }
  return { origin: "", credentials: "same-origin", headers: {} };
}

// The session is missing, expired or revoked: drop every cached answer and go to login, then
// back to this page.
async function handleUnauthorized(): Promise<void> {
  clearQueryCache();
  const path = safeNextPath(
    `${window.location.pathname}${window.location.search}`,
  );
  if (path !== null && isLoginPath(path)) return;
  window.location.assign(loginPathWithNext(path));
}

export const apiEnvironment: ApiEnvironment = {
  getRequestEnvironment,
  handleUnauthorized,
};
