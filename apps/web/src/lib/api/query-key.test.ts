import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";
import { parse } from "yaml";

import * as generated from "./generated";
import { apiQueryKey, orgQueryKeyPrefix } from "./query-key";

// The operations come from the contract itself, so a new GET endpoint is covered without
// touching this file.

type Parameter = { name: string; in: string };
type ParameterOrRef = Parameter | { $ref: string };
type Operation = { operationId?: string; parameters?: ParameterOrRef[] };
type PathItem = { get?: Operation; parameters?: ParameterOrRef[] };
type Spec = {
  paths: Record<string, PathItem>;
  components?: { parameters?: Record<string, Parameter> };
};

type GetOperation = {
  operationId: string;
  path: string;
  pathParams: string[];
  queryParams: string[];
};

const spec = parse(
  readFileSync(
    new URL("../../../../../api/openapi.yaml", import.meta.url),
    "utf8",
  ),
) as Spec;

function resolveParameter(parameter: ParameterOrRef): Parameter {
  if (!("$ref" in parameter)) return parameter;
  const name = parameter.$ref.replace("#/components/parameters/", "");
  const resolved = spec.components?.parameters?.[name];
  if (resolved === undefined) {
    throw new Error(`unresolved parameter ${parameter.$ref}`);
  }
  return resolved;
}

function getOperations(): GetOperation[] {
  const operations: GetOperation[] = [];
  for (const [path, item] of Object.entries(spec.paths)) {
    const operation = item.get;
    if (operation === undefined) continue;
    if (operation.operationId === undefined) {
      throw new Error(`GET ${path} has no operationId`);
    }
    const parameters = [
      ...(item.parameters ?? []),
      ...(operation.parameters ?? []),
    ].map(resolveParameter);
    // Orval takes path parameters in the order they appear in the path.
    const pathParams = [...path.matchAll(/\{([^}]+)\}/g)].map(
      (match) => match[1] ?? "",
    );
    for (const name of pathParams) {
      if (!parameters.some((p) => p.in === "path" && p.name === name)) {
        throw new Error(`GET ${path} does not declare path parameter ${name}`);
      }
    }
    operations.push({
      operationId: operation.operationId,
      path,
      pathParams,
      queryParams: parameters
        .filter((p) => p.in === "query")
        .map((p) => p.name),
    });
  }
  return operations;
}

type KeyFunction = (...args: unknown[]) => readonly unknown[];

function keyFunction(operationId: string): KeyFunction {
  const name = `get${operationId[0]?.toUpperCase()}${operationId.slice(1)}QueryKey`;
  const exported: unknown = (generated as Record<string, unknown>)[name];
  if (typeof exported !== "function") {
    throw new Error(
      `${name} is not exported by src/lib/api/generated for operation ${operationId}; run pnpm gen`,
    );
  }
  return exported as KeyFunction;
}

function sampleValues(
  operation: GetOperation,
  variant: string,
): Record<string, string> {
  const values: Record<string, string> = {};
  for (const name of operation.pathParams) values[name] = `${name}-${variant}`;
  return values;
}

function callKey(
  operation: GetOperation,
  pathValues: Record<string, string>,
  queryValue: string,
): readonly unknown[] {
  const args: unknown[] = operation.pathParams.map((name) => pathValues[name]);
  if (operation.queryParams.length > 0) {
    args.push(
      Object.fromEntries(
        operation.queryParams.map((name) => [name, queryValue]),
      ),
    );
  }
  return keyFunction(operation.operationId)(...args);
}

function resolvePath(path: string, values: Record<string, string>): string {
  return path.replace(/\{([^}]+)\}/g, (_, name: string) => values[name] ?? "");
}

const operations = getOperations();
const orgOperations = operations.filter((o) =>
  o.path.startsWith("/orgs/{orgId}"),
);
const userOperations = operations.filter(
  (o) => !o.path.startsWith("/orgs/{orgId}"),
);

describe("generated query keys", () => {
  it("finds org-scoped GET operations in the contract", () => {
    expect(orgOperations.length).toBeGreaterThan(0);
    expect(userOperations.map((o) => o.operationId)).toContain("getMe");
  });

  it("has a generated key function for every GET operation", () => {
    for (const operation of operations) {
      expect(() => keyFunction(operation.operationId)).not.toThrow();
    }
  });

  describe.each(orgOperations.map((o) => ({ ...o, name: o.operationId })))(
    "$name ($path)",
    (operation) => {
      const values = sampleValues(operation, "a");

      it("starts with the organization and the resolved path", () => {
        const key = callKey(operation, values, "q-a");
        expect(key).toHaveLength(4);
        expect(key[0]).toBe("org");
        expect(key[1]).toBe(values.orgId);
        expect(key[2]).toBe(resolvePath(operation.path, values));

        const params = key[3] as Record<string, unknown>;
        expect(params).not.toHaveProperty("orgId");
        for (const name of operation.pathParams) {
          if (name !== "orgId") expect(params[name]).toBe(values[name]);
        }
      });

      it("differs in the prefix when only the organization differs", () => {
        const a = callKey(operation, values, "q-a");
        const b = callKey(operation, { ...values, orgId: "orgId-b" }, "q-a");
        expect(b[1]).toBe("orgId-b");
        expect(b.slice(0, 2)).not.toEqual(a.slice(0, 2));
      });

      it.each(
        [...operation.pathParams.filter((p) => p !== "orgId"), "?query"].filter(
          (p) => p !== "?query" || operation.queryParams.length > 0,
        ),
      )("differs when %s differs", (name) => {
        const a = callKey(operation, values, "q-a");
        const b =
          name === "?query"
            ? callKey(operation, values, "q-b")
            : callKey(operation, { ...values, [name]: `${name}-b` }, "q-a");
        expect(b).not.toEqual(a);
        expect(b.slice(0, 2)).toEqual(a.slice(0, 2));
      });
    },
  );

  describe.each(userOperations.map((o) => ({ ...o, name: o.operationId })))(
    "$name ($path)",
    (operation) => {
      it("is a user key", () => {
        const values = sampleValues(operation, "a");
        const key = callKey(operation, values, "q-a");
        expect(key[0]).toBe("user");
        expect(key[1]).toBe(resolvePath(operation.path, values));
        expect(key).toHaveLength(3);
      });
    },
  );
});

describe("apiQueryKey", () => {
  it.each([
    {
      name: "a nested org path",
      properties: { orgId: "o1", tenantId: "t1" },
      url: "/orgs/o1/tenants/t1",
      want: ["org", "o1", "/orgs/o1/tenants/t1", { tenantId: "t1" }],
    },
    {
      name: "the org id at the end of the path",
      properties: { orgId: "o1" },
      url: "/orgs/o1",
      want: ["org", "o1", "/orgs/o1", {}],
    },
    {
      name: "a path with a query string",
      properties: { orgId: "o1", params: { page: 2 } },
      url: "/orgs/o1/tenants?page=2",
      want: ["org", "o1", "/orgs/o1/tenants?page=2", { params: { page: 2 } }],
    },
    {
      name: "a query string right after the org id",
      properties: {},
      url: "/orgs/o1?x=1",
      want: ["org", "o1", "/orgs/o1?x=1", {}],
    },
    {
      name: "a non-org path",
      properties: {},
      url: "/me",
      want: ["user", "/me", {}],
    },
    {
      name: "a path that only starts like /orgs",
      properties: { orgId: "o1" },
      url: "/orgsX/o1/members",
      want: ["user", "/orgsX/o1/members", { orgId: "o1" }],
    },
    {
      name: "/orgs without an id",
      properties: {},
      url: "/orgs/",
      want: ["user", "/orgs/", {}],
    },
  ])("keys $name", ({ properties, url, want }) => {
    expect(apiQueryKey(properties, { url })).toEqual(want);
  });

  it("takes the org id from the url, not from the properties", () => {
    expect(
      apiQueryKey({ orgId: "other" }, { url: "/orgs/o1/members" }),
    ).toEqual(["org", "o1", "/orgs/o1/members", {}]);
  });

  it("copies the properties", () => {
    const properties = { orgId: "o1", tenantId: "t1" };
    const key = apiQueryKey(properties, { url: "/orgs/o1/tenants/t1" });
    expect(key[3]).not.toBe(properties);
  });
});

describe("orgQueryKeyPrefix", () => {
  it("is the first two elements of an org key", () => {
    const key = apiQueryKey({ orgId: "o1" }, { url: "/orgs/o1/members" });
    expect(orgQueryKeyPrefix("o1")).toEqual(["org", "o1"]);
    expect(key.slice(0, 2)).toEqual(orgQueryKeyPrefix("o1"));
  });
});
