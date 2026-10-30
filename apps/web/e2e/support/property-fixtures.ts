import { expect, type Page, type TestInfo } from "@playwright/test";

import type {
  Property,
  PropertyKind,
  PropertyList,
} from "../../src/lib/api/generated/model";
import { openListPageOf } from "./list-pages";

// Properties the specs need only to exist (a place for their own unit, tenant and lease) are
// found by a stable name and created once, so a database reused across many runs does not grow
// by a property per test per run. Names start with "Wisma", after every property seed-demo
// makes in Griya Sejahtera, so the seeded properties keep the first page of the property list:
//
//   "Wisma Tetap <base> Satu|Dua"           reused by every run (this module)
//   "Wisma Uji <base> <run id> Satu|Dua"    made by a test that needs a fresh property of its
//                                           own (the spec builds the name)
//
// "Wisma Tetap" sorts before "Wisma Uji", so the reused set always stays inside the first 100
// properties, which is all the lease form's property picker loads, however many per-run ones pile
// up after it. A reused property collects one unit per run, so units made in it need a name no
// other run uses.

// One reused property per project, so the two sizes, which run in parallel, never share one.
export function stablePropertyName(testInfo: TestInfo, base: string): string {
  const project = testInfo.project.name === "phone" ? "Dua" : "Satu";
  return `Wisma Tetap ${base} ${project}`;
}

async function findProperty(
  page: Page,
  orgId: string,
  name: string,
): Promise<Property | undefined> {
  const response = await page.request.get(`/api/orgs/${orgId}/properties`, {
    params: { q: name, pageSize: 100 },
  });
  expect(response.status(), await response.text()).toBe(200);
  const list = (await response.json()) as PropertyList;
  // Names are unique per organization without regard to case.
  return list.items.find(
    (property) => property.name.toLowerCase() === name.toLowerCase(),
  );
}

// The property with the stable name for this test's base and project, made as the signed-in
// member (the page's cookies) when it does not exist yet. A second test asking for the same one
// at the same moment finds it after losing the race to create it.
export async function findOrCreateProperty(
  page: Page,
  orgId: string,
  testInfo: TestInfo,
  fixture: { base: string; kind: PropertyKind },
): Promise<Property> {
  const name = stablePropertyName(testInfo, fixture.base);
  const existing = await findProperty(page, orgId, name);
  if (existing !== undefined) return existing;

  const response = await page.request.post(`/api/orgs/${orgId}/properties`, {
    data: { name, kind: fixture.kind, address: "" },
  });
  if (response.status() === 201) return (await response.json()) as Property;
  const raced = await findProperty(page, orgId, name);
  if (raced !== undefined) return raced;
  expect(response.status(), await response.text()).toBe(201);
  throw new Error("unreachable");
}

// Opens the page of the property list that holds the named property: one a test has just made or
// renamed is rarely on page 1, since per-run properties sort after the seeded and reused ones.
// Reloads the page, so assert any toast first.
export async function openPropertyListPageOf(
  page: Page,
  name: string,
): Promise<void> {
  await openListPageOf(page, "properties", name);
}
