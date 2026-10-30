import { expect, type Page, type TestInfo } from "@playwright/test";

import {
  DEFAULT_PAGE_SIZE,
  PAGE_PARAM,
} from "../../src/components/client/list/utils";
import type {
  Property,
  PropertyKind,
  PropertyList,
} from "../../src/lib/api/generated/model";

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

// Where a property stands in the list as the API sorts it (by name, the screen's default), found
// by paging through the whole list, 100 at a time, until the exact name turns up.
async function positionOf(
  page: Page,
  orgId: string,
  name: string,
): Promise<number> {
  const pageSize = 100;
  for (let pageNumber = 1; ; pageNumber += 1) {
    const response = await page.request.get(`/api/orgs/${orgId}/properties`, {
      params: { sort: "name", page: pageNumber, pageSize },
    });
    expect(response.status(), await response.text()).toBe(200);
    const list = (await response.json()) as PropertyList;
    const index = list.items.findIndex((property) => property.name === name);
    if (index !== -1) return (pageNumber - 1) * pageSize + index;
    if (pageNumber * pageSize >= list.total) {
      throw new Error(`${name} is not in the property list`);
    }
  }
}

// Opens the page of the property list (from the signed-in member's organization, the screen
// the browser is on) that holds the named property. The list pages at DEFAULT_PAGE_SIZE and keeps
// its page in the URL, and per-run properties sort after the seeded and reused ones, so a property
// a test has just made or renamed is rarely on page 1. Reloads the page, so a toast on screen is
// gone: assert it first. Both projects add properties at the same time, so a property near a page
// boundary can move to the next page between the lookup and the page loading; when its row is not
// there, the lookup is made again (a few times).
export async function openListPageOf(page: Page, name: string): Promise<void> {
  const url = new URL(page.url());
  const orgId = /^\/o\/([0-9a-f-]{36})\/properties\/?$/.exec(url.pathname)?.[1];
  if (orgId === undefined) throw new Error("not on the property list");
  const row = page
    .getByRole("list", { name: "Daftar properti" })
    .getByRole("listitem")
    .filter({ hasText: name });
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    const position = await positionOf(page, orgId, name);
    const listPage = Math.floor(position / DEFAULT_PAGE_SIZE) + 1;
    if (listPage === 1) {
      url.searchParams.delete(PAGE_PARAM);
    } else {
      url.searchParams.set(PAGE_PARAM, String(listPage));
    }
    await page.goto(`${url.pathname}${url.search}`);
    const found = await row
      .waitFor({ state: "visible", timeout: 5000 })
      .then(() => true)
      .catch(() => false);
    if (found) return;
  }
}
