// The query key of every generated query (Orval's override.query.queryKey).
//
// - A path under /orgs/{orgId}/ gives ['org', orgId, url, params], so switching organization
//   removes ['org', <previous id>] and one organization's cache can never answer for another.
// - Anything else (/me) gives ['user', url, params]. Logging out clears the whole cache.
//
// `url` is the resolved request path without the /api prefix (`/orgs/<id>/members`); `params`
// holds every other input Orval passes (other path parameters and query parameters), so two
// requests that differ in any input never share a key. An undefined input is left out, so the
// key built without query parameters (getListPropertiesQueryKey(orgId)) partially matches every
// page and filter of that list, and invalidating it refreshes them all.

const ORG_PATH = /^\/orgs\/([^/?#]+)(?:[/?#]|$)/;

export type ApiQueryKey =
  | readonly ["org", string, string, Record<string, unknown>]
  | readonly ["user", string, Record<string, unknown>];

export function apiQueryKey(
  properties: Record<string, unknown>,
  context: { url: string; queryOptions?: unknown },
): ApiQueryKey {
  const orgId = ORG_PATH.exec(context.url)?.[1];
  if (orgId === undefined) {
    return ["user", context.url, definedInputs(properties, () => true)];
  }
  return [
    "org",
    orgId,
    context.url,
    definedInputs(properties, (name) => name !== "orgId"),
  ];
}

function definedInputs(
  properties: Record<string, unknown>,
  keep: (name: string) => boolean,
): Record<string, unknown> {
  const params: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(properties)) {
    if (value !== undefined && keep(name)) params[name] = value;
  }
  return params;
}

// The prefix that matches every query of one organization.
export function orgQueryKeyPrefix(orgId: string): readonly ["org", string] {
  return ["org", orgId];
}
