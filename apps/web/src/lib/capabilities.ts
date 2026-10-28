import type { Capability } from "@/lib/api/generated/model";

// True when the member may do `action` at least on some records. An action the API does not
// list is denied, like on the server.
export function canSee(
  capabilities: readonly Capability[],
  action: string,
): boolean {
  const capability = capabilities.find((c) => c.action === action);
  return capability?.access === "allow" || capability?.access === "conditional";
}
