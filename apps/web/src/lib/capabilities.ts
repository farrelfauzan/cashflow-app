import { Ability, type RawRuleOf } from "@casl/ability";

import type { Capability } from "@/lib/api/generated/model";

// What the signed-in member may do in the current organization, for showing and hiding
// controls. The rules come only from getOrgMe's capabilities, through `capabilityRules`; CASL
// does the matching. Visibility only: the API checks every request.
//
// An action "resource:verb" becomes CASL's action `verb` on the subject `resource`:
// `ability.can("manage", "property")`. "allow" and "conditional" both grant it ("conditional"
// means the API decides per record, so the control shows); "deny" and an action the API does
// not list grant nothing, like on the server. The plain `Ability` has no conditions matcher,
// so a rule with conditions cannot be built: the browser never evaluates policy conditions.
export type AppAbility = Ability<[string, string]>;

// "allow" also grants the action on `everywhere(resource)`: on every record, not just some.
// For controls the API accepts only for the whole organization, like adding a property:
// `ability.can("manage", everywhere("property"))`. The suffix reads like a policy's resource
// scope (`property/<id>`, so `property/*` is every property).
export function everywhere(resource: string): string {
  return `${resource}/*`;
}

// CASL's own wildcards ("manage" for any action, "all" for any subject) are moved to values no
// action can hold, so the API's "property:manage" is a plain action. The API's "*" and
// "invoice:*" stay plain strings too: they match only themselves, as an exact action match did
// before CASL.
const WILDCARD_OPTIONS = {
  anyAction: "\u0000any",
  anySubjectType: "\u0000all",
};

// An action that is not "resource:verb" ("*", or one with an empty part, which CASL would read
// as any subject or action) keeps its whole text, prefixed, as the verb, on a subject no real
// resource can be. It then matches only itself. A "/" in the resource would clash with
// `everywhere`.
const MALFORMED = "\u0000";

// Splits "resource:verb" at its first colon into CASL's [action, subject].
export function abilityArgs(action: string): [verb: string, resource: string] {
  const colon = action.indexOf(":");
  const resource = action.slice(0, colon);
  const verb = action.slice(colon + 1);
  if (
    colon === -1 ||
    resource === "" ||
    verb === "" ||
    resource.includes("/")
  ) {
    return [`${MALFORMED}${action}`, MALFORMED];
  }
  return [verb, resource];
}

export function capabilityRules(
  capabilities: readonly Capability[],
): RawRuleOf<AppAbility>[] {
  const rules: RawRuleOf<AppAbility>[] = [];
  // The first entry for an action decides, should the API ever list one twice.
  const seen = new Set<string>();
  for (const { action, access } of capabilities) {
    if (seen.has(action)) continue;
    seen.add(action);
    if (access === "deny") continue;
    const [verb, resource] = abilityArgs(action);
    rules.push({ action: verb, subject: resource });
    if (access === "allow") {
      rules.push({ action: verb, subject: everywhere(resource) });
    }
  }
  return rules;
}

export function createAppAbility(
  capabilities: readonly Capability[] = [],
): AppAbility {
  return new Ability<[string, string]>(
    capabilityRules(capabilities),
    WILDCARD_OPTIONS,
  );
}
