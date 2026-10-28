// The query key of every generated query (Orval's override.query.queryKey).
//
// - A path under /orgs/{orgId}/ gives ['org', orgId, url, params], so switching organization
//   removes ['org', <previous id>] and one organization's cache can never answer for another.
// - Anything else (/me) gives ['user', url, params]. Logging out clears the whole cache.
//
// `url` is the resolved request path without the /api prefix (`/orgs/<id>/members`); `params`
// holds every other input Orval passes (other path parameters and query parameters), so two
// requests that differ in any input never share a key.

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
    return ["user", context.url, { ...properties }];
  }

  const params: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(properties)) {
    if (name !== "orgId") params[name] = value;
  }
  return ["org", orgId, context.url, params];
}

// The prefix that matches every query of one organization.
export function orgQueryKeyPrefix(orgId: string): readonly ["org", string] {
  return ["org", orgId];
}
