import {
  AxiosError,
  AxiosHeaders,
  CanceledError,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from "axios";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getQueryClient } from "@/lib/query/query-client";

import { apiAxios, apiRequest } from "./client";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  INVALID_PATH_CODE,
  NETWORK_ERROR_CODE,
  NETWORK_ERROR_MESSAGE,
  UNKNOWN_ERROR_CODE,
} from "./errors";

type FakeLocation = {
  pathname: string;
  search: string;
  assign: ReturnType<typeof vi.fn>;
};

type Reply = (config: InternalAxiosRequestConfig) => Promise<AxiosResponse>;

// Stands in for axios's xhr and http adapters, so no test touches the network. Like them, it
// rejects a status outside validateStatus with an AxiosError that carries the response.
let adapter: ReturnType<typeof vi.fn<Reply>>;
let location: FakeLocation;
const realAdapter = apiAxios.defaults.adapter;

function respond(body: string, status: number): void {
  adapter.mockImplementation(async (config) => {
    const response: AxiosResponse = {
      data: body,
      status,
      statusText: "",
      headers: {},
      config,
    };
    if (config.validateStatus?.(status) ?? true) return response;
    throw new AxiosError(
      `Request failed with status code ${status}`,
      AxiosError.ERR_BAD_RESPONSE,
      config,
      {},
      response,
    );
  });
}

function sentConfig(): InternalAxiosRequestConfig {
  expect(adapter).toHaveBeenCalledTimes(1);
  return adapter.mock.calls[0]![0];
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
  adapter = vi.fn<Reply>();
  apiAxios.defaults.adapter = adapter;
  location = { pathname: "/o/org-1/members", search: "", assign: vi.fn() };
  vi.stubGlobal("window", { location });
});

afterEach(() => {
  apiAxios.defaults.adapter = realAdapter;
  vi.unstubAllGlobals();
});

describe("apiRequest request", () => {
  it("calls the same-origin /api path with the generated request and the caller's options", async () => {
    respond("{}", 200);

    await apiRequest(
      {
        url: "/orgs/org-1/members",
        method: "POST",
        headers: { "Content-Type": "application/json" },
        data: { name: "Budi Santoso" },
      },
      { headers: { "X-Trace": "abc" } },
    );

    const config = sentConfig();
    expect(config.baseURL).toBe("/api");
    expect(config.url).toBe("/orgs/org-1/members");
    expect(config.method).toBe("post");
    expect(config.data).toBe(JSON.stringify({ name: "Budi Santoso" }));
    expect(config.withCredentials).toBeFalsy();
    const headers = AxiosHeaders.from(config.headers);
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Content-Type")).toBe("application/json");
    expect(headers.get("X-Trace")).toBe("abc");
    expect(headers.has("Authorization")).toBe(false);
    expect(headers.has("Cookie")).toBe(false);
  });

  it("sends a text body as it is and repeats query parameters without brackets", async () => {
    respond("{}", 200);

    await apiRequest({
      url: "/orgs/org-1/imports/tenants",
      method: "POST",
      headers: { "Content-Type": "text/csv" },
      data: "nama,unit\nBudi,A1",
      params: { dryRun: false, status: ["aktif", "keluar"] },
    });

    const config = sentConfig();
    expect(config.data).toBe("nama,unit\nBudi,A1");
    expect(apiAxios.getUri(config)).toBe(
      "/api/orgs/org-1/imports/tenants?dryRun=false&status=aktif&status=keluar",
    );
  });

  it.each([
    {
      name: "an absolute url",
      url: "https://example.com/x",
      sent: "/api/https://example.com/x",
    },
    {
      name: "a protocol-relative url",
      url: "//example.com/x",
      sent: "/api/example.com/x",
    },
  ])("keeps $name under the API's base URL", async ({ url, sent }) => {
    respond("{}", 200);

    await apiRequest({ url: "/me" }, { url });

    expect(apiAxios.getUri(sentConfig())).toBe(sent);
  });

  it.each([
    { name: "a parent segment and a query", id: "../auth/me?x=" },
    { name: "a parent segment", id: "../auth" },
    { name: "a current segment", id: "." },
    { name: "a percent-encoded parent segment", id: "%2E%2e" },
    { name: "a half-encoded parent segment", id: ".%2e" },
    { name: "a backslash", id: "..\\auth" },
    { name: "a fragment", id: "org-1#x" },
    { name: "a tab between two dots", id: ".\t." },
    { name: "a newline between two dots", id: ".\n./auth" },
  ])("never sends a path value with $name", async ({ id }) => {
    const error = await caught(apiRequest({ url: `/orgs/${id}/me` }));

    expect(error).toMatchObject({
      status: 400,
      code: INVALID_PATH_CODE,
      message: GENERIC_ERROR_MESSAGE,
    });
    expect(adapter).not.toHaveBeenCalled();
  });

  it("checks the url a caller's options put in place of the generated one", async () => {
    const error = await caught(
      apiRequest({ url: "/me" }, { url: "/orgs/../auth/me" }),
    );

    expect(error.code).toBe(INVALID_PATH_CODE);
    expect(adapter).not.toHaveBeenCalled();
  });

  it.each(["0d6f6e7c-5b1a-4c44-9a37-2f0d0c9b6a11", "tok_a1B2-c3.d4", "..tok"])(
    "sends an ordinary path value (%s)",
    async (id) => {
      respond("{}", 200);

      await apiRequest({ url: `/t/${id}/status` });

      expect(apiAxios.getUri(sentConfig())).toBe(`/api/t/${id}/status`);
    },
  );

  it("never follows a redirect or uses a proxy from the environment", () => {
    expect(apiAxios.defaults.maxRedirects).toBe(0);
    expect(apiAxios.defaults.proxy).toBe(false);
  });
});

describe("apiRequest success", () => {
  it("parses a JSON body", async () => {
    respond(JSON.stringify({ items: [{ name: "Budi Santoso" }] }), 200);
    await expect(apiRequest({ url: "/me" })).resolves.toEqual({
      items: [{ name: "Budi Santoso" }],
    });
  });

  it.each([
    { name: "204", status: 204, body: "" },
    { name: "an empty 200", status: 200, body: "" },
  ])("returns undefined for $name", async ({ status, body }) => {
    respond(body, status);
    await expect(apiRequest({ url: "/auth/logout" })).resolves.toBeUndefined();
  });

  it("throws unknown for a non-JSON 2xx body", async () => {
    respond("nama,unit\nBudi,A1", 200);
    const error = await caught(apiRequest({ url: "/me" }));
    expect(error.code).toBe(UNKNOWN_ERROR_CODE);
    expect(error.status).toBe(200);
    expect(error.message).toBe(GENERIC_ERROR_MESSAGE);
  });
});

describe("apiRequest errors", () => {
  it("turns the API's error body into an ApiError", async () => {
    respond(jsonError("validation_failed", "Nomor tidak valid.", "phone"), 422);
    const error = await caught(apiRequest({ url: "/orgs/org-1/members" }));
    expect(error).toMatchObject({
      status: 422,
      code: "validation_failed",
      message: "Nomor tidak valid.",
      field: "phone",
    });
  });

  it("never shows a raw HTML body", async () => {
    const html = "<html><body>502 Bad Gateway nginx</body></html>";
    respond(html, 502);
    const error = await caught(apiRequest({ url: "/me" }));
    expect(error.status).toBe(502);
    expect(error.code).toBe(UNKNOWN_ERROR_CODE);
    expect(error.message).toBe(GENERIC_ERROR_MESSAGE);
    expect(error.message).not.toContain("Bad Gateway");
  });

  it("gives network_error with status 0 when no response arrives", async () => {
    adapter.mockImplementation(async (config) => {
      throw new AxiosError("Network Error", AxiosError.ERR_NETWORK, config, {});
    });
    const error = await caught(apiRequest({ url: "/me" }));
    expect(error.status).toBe(0);
    expect(error.code).toBe(NETWORK_ERROR_CODE);
    expect(error.message).toBe(NETWORK_ERROR_MESSAGE);
  });

  it("rethrows an abort unchanged", async () => {
    const abort = new CanceledError();
    adapter.mockRejectedValue(abort);
    await expect(apiRequest({ url: "/me" })).rejects.toBe(abort);
  });

  it("passes an error raised before sending through unchanged", async () => {
    // The browser client rendered on the server: no window.
    vi.unstubAllGlobals();
    await expect(apiRequest({ url: "/me" })).rejects.toThrow(
      /cannot run during server rendering/,
    );
    expect(adapter).not.toHaveBeenCalled();
  });

  it("does not navigate on a 403", async () => {
    respond(jsonError("forbidden", "Anda tidak punya akses."), 403);
    const error = await caught(apiRequest({ url: "/orgs/org-1/members" }));
    expect(error.status).toBe(403);
    expect(location.assign).not.toHaveBeenCalled();
  });
});

describe("apiRequest on 401", () => {
  it("clears the cache, goes to login with the current path and still throws", async () => {
    location.pathname = "/o/org-1/members";
    location.search = "?status=aktif";
    const queryClient = getQueryClient();
    queryClient.setQueryData(["org", "org-1", "/orgs/org-1/members", {}], []);
    queryClient.setQueryData(["user", "/me", {}], { name: "Siti Rahayu" });
    respond(jsonError("unauthenticated", "Sesi berakhir. Masuk lagi."), 401);

    const error = await caught(apiRequest({ url: "/orgs/org-1/members" }));

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
    respond(jsonError("unauthenticated", "Sesi berakhir. Masuk lagi."), 401);

    const error = await caught(apiRequest({ url: "/t/secret-token/status" }));

    expect(error.status).toBe(401);
    expect(location.assign).not.toHaveBeenCalled();
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
  });

  it("does not navigate when already on the login page", async () => {
    location.pathname = "/login";
    location.search = "?next=%2Forganizations";
    respond(jsonError("unauthenticated", "Sesi berakhir. Masuk lagi."), 401);

    const error = await caught(apiRequest({ url: "/me" }));

    expect(error.status).toBe(401);
    expect(location.assign).not.toHaveBeenCalled();
  });
});
