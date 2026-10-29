import {
  getRedirectUrl,
  unstable_doesMiddlewareMatch,
} from "next/experimental/testing/server";
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";

import { config, proxy } from "./proxy";

const ORIGIN = "http://localhost:3000";

function request(path: string, headers: Record<string, string> = {}) {
  return new NextRequest(`${ORIGIN}${path}`, { headers });
}

// NextResponse.next({ request: { headers } }) passes the new request headers to Next.js
// through these response headers.
function forwardedRequestHeader(response: Response, name: string) {
  const overridden = response.headers.get("x-middleware-override-headers");
  return {
    overridden: overridden?.split(",") ?? [],
    value: response.headers.get(`x-middleware-request-${name}`),
  };
}

describe("proxy at the root", () => {
  it.each([
    { name: "without a session", path: "/", cookie: "", want: "/login" },
    {
      name: "with an empty cookie",
      path: "/",
      cookie: "cf_session=",
      want: "/login",
    },
    {
      name: "with a session",
      path: "/",
      cookie: "cf_session=tok",
      want: "/organizations",
    },
    {
      name: "with a session and a query",
      path: "/?next=%2F%2Fevil.com",
      cookie: "cf_session=tok",
      want: "/organizations",
    },
  ])("redirects $name to $want", ({ path, cookie, want }) => {
    const response = proxy(request(path, cookie ? { cookie } : {}));
    expect(response.status).toBe(307);
    expect(getRedirectUrl(response)).toBe(`${ORIGIN}${want}`);
  });
});

describe("proxy without a session", () => {
  it.each([
    {
      name: "the organizations page",
      path: "/organizations",
      want: "/login?next=%2Forganizations",
    },
    {
      name: "an org page with a query",
      path: "/o/org-1/members?status=aktif",
      want: "/login?next=%2Fo%2Forg-1%2Fmembers%3Fstatus%3Daktif",
    },
  ])("redirects $name to login", ({ path, want }) => {
    const response = proxy(request(path));
    expect(response.status).toBe(307);
    expect(getRedirectUrl(response)).toBe(`${ORIGIN}${want}`);
  });

  it("treats an empty cookie as missing", () => {
    const response = proxy(
      request("/organizations", { cookie: "cf_session=" }),
    );
    expect(response.status).toBe(307);
  });

  it("ignores other cookies", () => {
    const response = proxy(request("/organizations", { cookie: "other=1" }));
    expect(response.status).toBe(307);
  });
});

describe("proxy with a session", () => {
  it("lets the request through and passes the path on", () => {
    const response = proxy(
      request("/o/org-1/members?status=aktif", { cookie: "cf_session=tok" }),
    );
    expect(getRedirectUrl(response)).toBeNull();
    expect(response.headers.get("x-middleware-next")).toBe("1");
    const header = forwardedRequestHeader(response, "x-request-path");
    expect(header.overridden).toContain("x-request-path");
    expect(header.value).toBe("/o/org-1/members?status=aktif");
  });

  it("overwrites a client-supplied x-request-path", () => {
    const response = proxy(
      request("/organizations", {
        cookie: "cf_session=tok",
        "x-request-path": "//evil.com",
      }),
    );
    expect(forwardedRequestHeader(response, "x-request-path").value).toBe(
      "/organizations",
    );
  });

  it("keeps the other request headers", () => {
    const response = proxy(
      request("/organizations", {
        cookie: "cf_session=tok",
        "accept-language": "id-ID",
      }),
    );
    expect(forwardedRequestHeader(response, "accept-language").value).toBe(
      "id-ID",
    );
  });
});

describe("proxy matcher", () => {
  it.each([
    { path: "/organizations", want: true },
    { path: "/o/org-1", want: true },
    { path: "/o/org-1/members", want: true },
    { path: "/o/org-1/tenants/tenant-1", want: true },
    { path: "/login", want: false },
    { path: "/login?next=%2Forganizations", want: false },
    { path: "/t/secret-token", want: false },
    { path: "/api/me", want: false },
    { path: "/api/orgs/org-1/members", want: false },
    { path: "/", want: true },
    { path: "/?utm=1", want: true },
  ])("matches $path: $want", ({ path, want }) => {
    expect(
      unstable_doesMiddlewareMatch({ config, url: `${ORIGIN}${path}` }),
    ).toBe(want);
  });
});
