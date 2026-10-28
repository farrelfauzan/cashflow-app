import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import type { Capability, CapabilityAccess } from "@/lib/api/generated/model";

import {
  abilityArgs,
  createAppAbility,
  everywhere,
  type AppAbility,
} from "./capabilities";

// The checks CASL replaced, copied as they were (src/lib/capabilities.ts and
// components/client/property/utils.ts before T-029). The adapter must answer exactly what they
// answered, for every action.
function oldCanSee(capabilities: readonly Capability[], action: string) {
  const capability = capabilities.find((c) => c.action === action);
  return capability?.access === "allow" || capability?.access === "conditional";
}
function oldCanDoEverywhere(
  capabilities: readonly Capability[],
  action: string,
) {
  return capabilities.find((c) => c.action === action)?.access === "allow";
}

// Every action the API knows, read from its catalog, so a new action is covered without
// touching this test.
function catalogActions(): string[] {
  const file = fileURLToPath(
    new URL(
      "../../../../services/api/internal/authz/catalog.go",
      import.meta.url,
    ),
  );
  const source = readFileSync(file, "utf8");
  const start = source.indexOf("var catalog = map[string][]string{");
  const block = source.slice(start, source.indexOf("\n}\n", start));
  const actions: string[] = [];
  for (const [, resource, verbs] of block.matchAll(
    /^\s*"([a-z-]+)":\s*\{([^}]*)\}/gm,
  )) {
    for (const [, verb] of (verbs ?? "").matchAll(/"([a-z-]+)"/g)) {
      actions.push(`${resource}:${verb}`);
    }
  }
  return actions;
}

const CATALOG = catalogActions();

// Wildcards a policy may name, CASL's own keywords as the API might spell them, and odd strings
// the API never sends but the adapter must not misread.
const EXTRA = [
  "*",
  "*:read",
  "invoice:*",
  "property:*",
  "all:read",
  "property:all",
  "manage:property",
  "visit:fly",
  "",
  "member:",
  ":read",
  "a:b:c",
  "property/*:manage",
];

const ACTIONS = [...CATALOG, ...EXTRA];
const ACCESSES: CapabilityAccess[] = ["allow", "conditional", "deny"];

function expectSameAnswers(
  ability: AppAbility,
  capabilities: readonly Capability[],
) {
  for (const action of ACTIONS) {
    const [verb, resource] = abilityArgs(action);
    expect(ability.can(verb, resource), `can ${action}`).toBe(
      oldCanSee(capabilities, action),
    );
    expect(
      ability.can(verb, everywhere(resource)),
      `can ${action} everywhere`,
    ).toBe(oldCanDoEverywhere(capabilities, action));
  }
}

// A deterministic spread of accesses over the whole catalog, like a real getOrgMe answer.
function mixedCapabilities(seed: number): Capability[] {
  let state = seed;
  return ACTIONS.flatMap((action): Capability[] => {
    state = (state * 1103515245 + 12345) % 2147483648;
    const pick = state % 4;
    const access = ACCESSES[pick];
    return access === undefined ? [] : [{ action, access }];
  });
}

describe("createAppAbility answers what canSee and canDoEverywhere answered", () => {
  it("reads the whole catalog", () => {
    expect(CATALOG.length).toBeGreaterThanOrEqual(40);
    expect(CATALOG).toContain("property:manage");
    expect(CATALOG).toContain("invoice:send-reminder");
  });

  it("with no capabilities", () => {
    expectSameAnswers(createAppAbility([]), []);
    expectSameAnswers(createAppAbility(), []);
  });

  it.each(ACCESSES)("with each action alone as %s", (access) => {
    for (const action of ACTIONS) {
      const capabilities = [{ action, access }];
      expectSameAnswers(createAppAbility(capabilities), capabilities);
    }
  });

  it.each([1, 2, 3, 4, 5, 6, 7, 8])(
    "with a mixed answer for the whole catalog (seed %i)",
    (seed) => {
      const capabilities = mixedCapabilities(seed);
      expectSameAnswers(createAppAbility(capabilities), capabilities);
    },
  );

  it("when an action is listed twice, the first entry decides", () => {
    const capabilities: Capability[] = [
      { action: "member:invite", access: "deny" },
      { action: "member:invite", access: "allow" },
      { action: "unit:manage", access: "conditional" },
      { action: "unit:manage", access: "deny" },
    ];
    expectSameAnswers(createAppAbility(capabilities), capabilities);
  });

  it("after ability.update, as on an organization switch", () => {
    const first = mixedCapabilities(11);
    const second = mixedCapabilities(12);
    const ability = createAppAbility(first);
    expectSameAnswers(ability, first);
    ability.update(createAppAbility(second).rules);
    expectSameAnswers(ability, second);
    ability.update([]);
    expectSameAnswers(ability, []);
  });
});

describe("createAppAbility", () => {
  it("does not treat the API's 'manage' as CASL's any action", () => {
    const ability = createAppAbility([
      { action: "property:manage", access: "allow" },
    ]);
    expect(ability.can("manage", "property")).toBe(true);
    expect(ability.can("read", "property")).toBe(false);
  });

  it("does not treat a wildcard as matching other actions", () => {
    const ability = createAppAbility([
      { action: "invoice:*", access: "allow" },
      { action: "*", access: "allow" },
    ]);
    expect(ability.can("read", "invoice")).toBe(false);
    expect(ability.can("read", "property")).toBe(false);
  });

  it("grants 'conditional' on some records but not everywhere", () => {
    const ability = createAppAbility([
      { action: "property:read", access: "conditional" },
    ]);
    expect(ability.can("read", "property")).toBe(true);
    expect(ability.can("read", everywhere("property"))).toBe(false);
  });
});
