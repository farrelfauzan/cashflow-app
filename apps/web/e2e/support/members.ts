import { expect, type Page } from "@playwright/test";

import { ORG_HOME_URL } from "./shell";

// Opens "Anggota tim" of an organization, picked by name, and waits for its list.
export async function openMembers(page: Page, organization: string) {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: organization })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/members`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Anggota tim" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar anggota" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}
