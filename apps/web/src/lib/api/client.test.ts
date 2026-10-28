import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getQueryClient } from "@/lib/query/query-client";

import { apiFetch } from "./client";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
  NETWORK_ERROR_MESSAGE,
  UNKNOWN_ERROR_CODE,
} from "./errors";

type FakeLocation = {
  pathname: string;
  search: string;
  assign: ReturnType<typeof vi.fn>;
};

let fetchMock: ReturnType<typeof vi.fn>;
let location: FakeLocation;

function respond(body: string | null, init: ResponseInit = {}): void {
  fetchMock.mockResolvedValue(new Response(body, init));
}

function jsonError(code: string, message: string, field?: string): string {
  return JSON.stringify({ error: { code, message, field } });
}

async function caught(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise;
  } catch (error) {
    if (error instanceof ApiError) return error;
    throw error;
  }
  throw new Error("expected the call to throw");
}

beforeEach(() => {
  fetchMock = vi.fn();
  location = { pathname: "/o/org-1/members", search: "", assign: vi.fn() };
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { location });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("apiFetch request", () => {
  it("calls the same-origin /api path with the client's options", async () => {
    respond("{}", { status: 200 });

    await apiFetch("/orgs/org-1/members", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Trace": "abc" },
      body: "{}",
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/orgs/org-1/members");
    expect(init.method).toBe("POST");
    expect(init.body).toBe("{}");
    expect(init.credentials).toBe("same-origin");
    expect(init.cache).toBe("no-store");
    const headers = new Headers(init.headers);
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Trace")).toBe("abc");
    expect(headers.has("Authorization")).toBe(false);
  });
});

describe("apiFetch success", () => {
  it("parses a JSON body", async () => {
    respond(JSON.stringify({ items: [{ name: "Budi Santoso" }] }), {
      status: 200,
    });
    await expect(apiFetch("/me")).resolves.toEqual({
      items: [{ name: "Budi Santoso" }],
    });
  });

  it.each([
    { name: "204", status: 204, body: null },
    { name: "an empty 200", status: 200, body: "" },
  ])("returns undefined for $name", async ({ status, body }) => {
    respond(body, { status });
    await expect(apiFetch("/auth/logout")).resolves.toBeUndefined();
  });

  it("throws unknown for a non-JSON 2xx body", async () => {
    respond("nama,unit\nBudi,A1", { status: 200 });
    const error = await caught(apiFetch("/me"));
    expect(error.code).toBe(UNKNOWN_ERROR_CODE);
    expect(error.status).toBe(200);
    expect(error.message).toBe(GENERIC_ERROR_MESSAGE);
  });
});

describe("apiFetch errors", () => {
  it("turns the API's error body into an ApiError", async () => {
    respond(jsonError("validation_failed", "Nomor tidak valid.", "phone"), {
      status: 422,
    });
    const error = await caught(apiFetch("/orgs/org-1/members"));
    expect(error).toMatchObject({
      status: 422,
      code: "validation_failed",
      message: "Nomor tidak valid.",
      field: "phone",
    });
  });

  it("never shows a raw HTML body", async () => {
    const html = "<html><body>502 Bad Gateway nginx</body></html>";
    respond(html, { status: 502 });
    const error = await caught(apiFetch("/me"));
    expect(error.status).toBe(502);
    expect(error.code).toBe(UNKNOWN_ERROR_CODE);
    expect(error.message).toBe(GENERIC_ERROR_MESSAGE);
    expect(error.message).not.toContain("Bad Gateway");
  });

  it("gives network_error with status 0 when fetch rejects", async () => {
    fetchMock.mockRejectedValue(new TypeError("Failed to fetch"));
    const error = await caught(apiFetch("/me"));
    expect(error.status).toBe(0);
    expect(error.code).toBe(NETWORK_ERROR_CODE);
    expect(error.message).toBe(NETWORK_ERROR_MESSAGE);
  });

  it("rethrows an abort unchanged", async () => {
    const abort = new DOMException("The operation was aborted.", "AbortError");
    fetchMock.mockRejectedValue(abort);
    await expect(apiFetch("/me")).rejects.toBe(abort);
  });

  it("does not navigate on a 403", async () => {
    respond(jsonError("forbidden", "Anda tidak punya akses."), { status: 403 });
    const error = await caught(apiFetch("/orgs/org-1/members"));
    expect(error.status).toBe(403);
    expect(location.assign).not.toHaveBeenCalled();
  });
});

describe("apiFetch on 401", () => {
  it("clears the cache, goes to login with the current path and still throws", async () => {
    location.pathname = "/o/org-1/members";
    location.search = "?status=aktif";
    const queryClient = getQueryClient();
    queryClient.setQueryData(["org", "org-1", "/orgs/org-1/members", {}], []);
    queryClient.setQueryData(["user", "/me", {}], { name: "Siti Rahayu" });
    respond(jsonError("unauthenticated", "Sesi berakhir. Masuk lagi."), {
      status: 401,
    });

    const error = await caught(apiFetch("/orgs/org-1/members"));

    expect(error.status).toBe(401);
    expect(error.code).toBe("unauthenticated");
    expect(location.assign).toHaveBeenCalledTimes(1);
    expect(location.assign).toHaveBeenCalledWith(
      "/login?next=%2Fo%2Forg-1%2Fmembers%3Fstatus%3Daktif",
    );
    expect(getQueryClient()).toBe(queryClient);
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
  });

  it("stays on a tenant page and keeps its token out of any URL", async () => {
    location.pathname = "/t/secret-token";
    location.search = "";
    const queryClient = getQueryClient();
    queryClient.setQueryData(["user", "/me", {}], { name: "Siti Rahayu" });
    respond(jsonError("unauthenticated", "Sesi berakhir. Masuk lagi."), {
      status: 401,
    });

    const error = await caught(apiFetch("/t/secret-token/status"));

    expect(error.status).toBe(401);
    expect(location.assign).not.toHaveBeenCalled();
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
  });

  it("does not navigate when already on the login page", async () => {
    location.pathname = "/login";
    location.search = "?next=%2Forganizations";
    respond(jsonError("unauthenticated", "Sesi berakhir. Masuk lagi."), {
      status: 401,
    });

    const error = await caught(apiFetch("/me"));

    expect(error.status).toBe(401);
    expect(location.assign).not.toHaveBeenCalled();
  });
});
