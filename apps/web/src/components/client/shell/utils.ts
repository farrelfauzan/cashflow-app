import { ApiError } from "@/lib/api/errors";
import type { Capability } from "@/lib/api/generated/model";

const NO_CAPABILITIES: readonly Capability[] = [];

// The capabilities the ability follows: getOrgMe's latest answer, or the ones the server loaded
// with the page until the browser has its own. A 401 means the session is gone, so nothing is
// granted while the API client sends the member to login.
export function currentCapabilities(
  fromServer: readonly Capability[],
  fromQuery: readonly Capability[] | undefined,
  error: unknown,
): readonly Capability[] {
  if (error instanceof ApiError && error.status === 401) return NO_CAPABILITIES;
  return fromQuery ?? fromServer;
}
