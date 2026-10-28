import { QueryClient } from "@tanstack/react-query";

import { orgQueryKeyPrefix } from "@/lib/api/query-key";
import { ApiError } from "@/lib/api/errors";

// Retry a query once when the API could not answer (no response, or a 5xx). A 4xx is an answer:
// retrying a 401, 403 or 404 gives the same result. Other errors are bugs and are not retried.
export function shouldRetryQuery(
  failureCount: number,
  error: unknown,
): boolean {
  if (failureCount >= 1) return false;
  if (!(error instanceof ApiError)) return false;
  return error.status === 0 || error.status >= 500;
}

export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: shouldRetryQuery,
      },
      // A write is never repeated on its own; the screen offers "Coba lagi".
      mutations: { retry: false },
    },
  });
}

let browserQueryClient: QueryClient | undefined;

// One client for the life of the page in the browser; a fresh one per server render so no
// request ever sees another request's data.
export function getQueryClient(): QueryClient {
  if (typeof window === "undefined") return makeQueryClient();
  browserQueryClient ??= makeQueryClient();
  return browserQueryClient;
}

// Logging out, or a 401: forget every cached answer, of every organization.
export function clearQueryCache(): void {
  getQueryClient().clear();
}

// Leaving an organization: forget everything cached under ['org', orgId].
export function removeOrgQueries(
  queryClient: QueryClient,
  orgId: string,
): void {
  queryClient.removeQueries({ queryKey: orgQueryKeyPrefix(orgId) });
}
