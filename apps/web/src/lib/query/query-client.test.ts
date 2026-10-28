import { QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import { apiQueryKey } from "@/lib/api/query-key";

import {
  makeQueryClient,
  removeOrgQueries,
  shouldRetryQuery,
} from "./query-client";

function apiError(status: number): ApiError {
  return new ApiError({ status, code: "x", message: "Pesan." });
}

describe("shouldRetryQuery", () => {
  it.each([
    { name: "a network error", failures: 0, error: apiError(0), want: true },
    { name: "a 500", failures: 0, error: apiError(500), want: true },
    { name: "a 503", failures: 0, error: apiError(503), want: true },
    {
      name: "a network error again",
      failures: 1,
      error: apiError(0),
      want: false,
    },
    { name: "a 500 again", failures: 1, error: apiError(500), want: false },
    {
      name: "a 500 a third time",
      failures: 2,
      error: apiError(500),
      want: false,
    },
    { name: "a 400", failures: 0, error: apiError(400), want: false },
    { name: "a 401", failures: 0, error: apiError(401), want: false },
    { name: "a 403", failures: 0, error: apiError(403), want: false },
    { name: "a 404", failures: 0, error: apiError(404), want: false },
    { name: "a 422", failures: 0, error: apiError(422), want: false },
    { name: "a 429", failures: 0, error: apiError(429), want: false },
    {
      name: "a plain Error",
      failures: 0,
      error: new Error("bug"),
      want: false,
    },
    {
      name: "an error shaped like ApiError",
      failures: 0,
      error: { status: 500, code: "x", message: "Pesan." },
      want: false,
    },
    { name: "undefined", failures: 0, error: undefined, want: false },
  ])("retries $name: $want", ({ failures, error, want }) => {
    expect(shouldRetryQuery(failures, error)).toBe(want);
  });
});

describe("makeQueryClient", () => {
  it("uses shouldRetryQuery for queries and never retries mutations", () => {
    const defaults = makeQueryClient().getDefaultOptions();
    expect(defaults.queries?.retry).toBe(shouldRetryQuery);
    expect(defaults.mutations?.retry).toBe(false);
  });
});

describe("removeOrgQueries", () => {
  it("removes only that organization's queries", () => {
    const queryClient = new QueryClient();
    const orgA = apiQueryKey(
      { orgId: "org-a" },
      { url: "/orgs/org-a/members" },
    );
    const orgATenant = apiQueryKey(
      { orgId: "org-a", tenantId: "t-1" },
      { url: "/orgs/org-a/tenants/t-1" },
    );
    const orgB = apiQueryKey(
      { orgId: "org-b" },
      { url: "/orgs/org-b/members" },
    );
    const user = apiQueryKey({}, { url: "/me" });
    queryClient.setQueryData(orgA, ["Budi Santoso"]);
    queryClient.setQueryData(orgATenant, { name: "Siti Rahayu" });
    queryClient.setQueryData(orgB, ["Dewi Lestari"]);
    queryClient.setQueryData(user, { name: "Agus Wijaya" });

    removeOrgQueries(queryClient, "org-a");

    expect(queryClient.getQueryData(orgA)).toBeUndefined();
    expect(queryClient.getQueryData(orgATenant)).toBeUndefined();
    expect(queryClient.getQueryData(orgB)).toEqual(["Dewi Lestari"]);
    expect(queryClient.getQueryData(user)).toEqual({ name: "Agus Wijaya" });
    expect(queryClient.getQueryCache().getAll()).toHaveLength(2);
  });
});
