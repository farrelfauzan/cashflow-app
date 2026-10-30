import { expect, type Page } from "@playwright/test";

import {
  DEFAULT_PAGE_SIZE,
  PAGE_PARAM,
} from "../../src/components/client/list/utils";
import type {
  MemberList,
  PropertyList,
} from "../../src/lib/api/generated/model";

// The list screens a spec may need to find a row on. Both sort by name by default and page at
// DEFAULT_PAGE_SIZE, with the page in the URL, so a row that sorts after many others (specs name
// what they make so it sorts after every seeded row) is on a later page once enough rows pile up
// in a database reused across runs.
const SCREENS = {
  properties: { listName: "Daftar properti", segment: "properties" },
  members: { listName: "Daftar anggota", segment: "members" },
} as const;

export type ListScreen = keyof typeof SCREENS;

type Named = { name: string };

// Where a row stands in the list as the API sorts it (by name, the screens' default), found by
// paging through the whole list, 100 at a time, until the exact name turns up.
async function positionOf(
  page: Page,
  orgId: string,
  screen: ListScreen,
  name: string,
): Promise<number> {
  const pageSize = 100;
  for (let pageNumber = 1; ; pageNumber += 1) {
    const response = await page.request.get(
      `/api/orgs/${orgId}/${SCREENS[screen].segment}`,
      { params: { sort: "name", page: pageNumber, pageSize } },
    );
    expect(response.status(), await response.text()).toBe(200);
    const list = (await response.json()) as PropertyList | MemberList;
    const items: Named[] = list.items;
    const index = items.findIndex((item) => item.name === name);
    if (index !== -1) return (pageNumber - 1) * pageSize + index;
    if (pageNumber * pageSize >= list.total) {
      throw new Error(`${name} is not in the ${screen} list`);
    }
  }
}

// Opens the page of the screen's list (the organization and screen the browser is on) that holds
// the row with this exact name. Reloads the page, so a toast on screen is gone: assert it first.
// Both projects add rows at the same time, so a row near a page boundary can move to the next
// page between the lookup and the page loading; when its row is not there, the lookup is made
// again (a few times).
export async function openListPageOf(
  page: Page,
  screen: ListScreen,
  name: string,
): Promise<void> {
  const { listName, segment } = SCREENS[screen];
  const url = new URL(page.url());
  const match = new RegExp(`^/o/([0-9a-f-]{36})/${segment}/?$`).exec(
    url.pathname,
  );
  const orgId = match?.[1];
  if (orgId === undefined) throw new Error(`not on the ${screen} list`);
  const row = page
    .getByRole("list", { name: listName })
    .getByRole("listitem")
    .filter({ hasText: name });
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    const position = await positionOf(page, orgId, screen, name);
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
  throw new Error(
    `${name} was not on the page of the ${screen} list where the API put it, after 3 tries`,
  );
}
