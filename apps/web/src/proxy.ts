import { NextResponse, type NextRequest } from "next/server";

import {
  LOGIN_PATH,
  loginPathWithNext,
  safeNextPath,
} from "@/lib/auth/next-path";
import { DEFAULT_POST_LOGIN_PATH } from "@/lib/auth/post-login-path";
import { REQUEST_PATH_HEADER, SESSION_COOKIE } from "@/lib/auth/session";

// The session gate for staff routes, an optimistic check only. Sessions are opaque tokens in
// PostgreSQL, so there is nothing to decode here and no API call: a missing cf_session cookie
// goes to /login?next=<path>; an expired or revoked one is caught by the API client's 401
// handler. Gating is not authorization: the API checks every request.
export function proxy(request: NextRequest) {
  const hasSession = Boolean(request.cookies.get(SESSION_COOKIE)?.value);

  // The app has no home page: "/" opens the organization list, or login without a session.
  if (request.nextUrl.pathname === "/") {
    const target = hasSession ? DEFAULT_POST_LOGIN_PATH : LOGIN_PATH;
    return NextResponse.redirect(new URL(target, request.url));
  }

  const path = safeNextPath(
    `${request.nextUrl.pathname}${request.nextUrl.search}`,
  );

  if (!hasSession) {
    return NextResponse.redirect(new URL(loginPathWithNext(path), request.url));
  }

  // Server components cannot read the URL, so pass the path on for the server-side 401
  // handler (src/lib/api/environment-server.ts). Always overwritten, never taken from the
  // client.
  const headers = new Headers(request.headers);
  headers.delete(REQUEST_PATH_HEADER);
  if (path !== null) headers.set(REQUEST_PATH_HEADER, path);
  return NextResponse.next({ request: { headers } });
}

// "/" and the staff routes only. /login, /t/... (tenant pages, the token is the access),
// /api/... (the Go API) and static files stay outside.
export const config = {
  matcher: ["/", "/organizations", "/o/:path*"],
};
