import { expect, test } from "@playwright/test";

import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { menuSections, ORG_HOME_URL, openMenu } from "./support/shell";
import { HENDRA, SARI } from "./support/staff";

// The organization picker against the real Go API: the session gate in src/proxy.ts, the
// server-side API call that forwards cf_session, and the API's 401 sent back to login.

const LOGIN_WITH_NEXT = "/login?next=%2Forganizations";

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("visiting /organizations goes to login with the return path", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);

    await page.goto("/organizations");

    await expect(page).toHaveURL(LOGIN_WITH_NEXT);
    await attachFullPageScreenshot(page, testInfo, "signed-out");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("member of two organizations", () => {
  // Saved by e2e/auth.setup.ts through the real login code flow.
  test.use({ storageState: HENDRA.storageState });

  test("picks an organization, then switches to the other from the menu", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);

    await page.goto("/organizations");

    await expect(
      page.getByRole("heading", { level: 1, name: "Pilih usaha" }),
    ).toBeVisible();
    await expect(page.getByText(HENDRA.name)).toBeVisible();
    await expect(page.getByText(HENDRA.phone)).toBeVisible();
    await expect(page.getByText("Anda terdaftar di 2 usaha.")).toBeVisible();
    // The API orders them by display name.
    const choices = page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link");
    await expect(choices).toHaveCount(HENDRA.organizations.length);
    for (const [index, name] of HENDRA.organizations.entries()) {
      await expect(choices.nth(index)).toHaveAccessibleName(name);
    }
    for (const choice of await choices.all()) {
      const box = await choice.boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    await attachFullPageScreenshot(page, testInfo, "organization-picker");

    // Admin in Griya Sejahtera.
    await choices.getByText("Griya Sejahtera").click();
    await expect(page).toHaveURL(ORG_HOME_URL);
    const griya = page.url();
    let menu = await openMenu(page, testInfo);
    await expect(menu.getByText("Griya Sejahtera")).toBeVisible();
    await expect(menu.getByText("Admin", { exact: true })).toBeVisible();

    // Supervisor in Kost Putri Anggrek.
    await menu.getByRole("link", { name: "Ganti usaha" }).click();
    await expect(page).toHaveURL("/organizations");
    await choices.getByText("Kost Putri Anggrek").click();
    await expect(page).toHaveURL(ORG_HOME_URL);
    expect(page.url()).not.toBe(griya);
    menu = await openMenu(page, testInfo);
    await expect(menu.getByText("Kost Putri Anggrek")).toBeVisible();
    await expect(menu.getByText("Supervisor", { exact: true })).toBeVisible();
    await expect(menuSections(menu)).toHaveText([
      "Beranda",
      "Properti",
      "Penyewa",
      "Anggota tim",
    ]);
    expect(outsideRequests).toEqual([]);
  });

  test("an organization the member is not in goes back to the picker", async ({
    page,
  }) => {
    await page.goto("/o/00000000-0000-4000-8000-000000000000");
    await expect(page).toHaveURL("/organizations");
    await expect(
      page.getByRole("heading", { level: 1, name: "Pilih usaha" }),
    ).toBeVisible();
  });
});

test.describe("member of one organization", () => {
  test.use({ storageState: SARI.storageState });

  test("goes straight in, with no organization switch", async ({
    page,
  }, testInfo) => {
    await page.goto("/organizations");

    await expect(page).toHaveURL(ORG_HOME_URL);
    const menu = await openMenu(page, testInfo);
    await expect(menu.getByText("Griya Sejahtera")).toBeVisible();
    await expect(menu.getByRole("link", { name: "Ganti usaha" })).toHaveCount(
      0,
    );
  });
});

test.describe("session the API rejects", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("a bogus cf_session cookie ends at login with the return path", async ({
    page,
    context,
    baseURL,
  }, testInfo) => {
    // Passes the cookie-presence gate in src/proxy.ts; the API answers 401 to the server
    // component's call, and the server-side handler redirects.
    await context.addCookies([
      { name: "cf_session", value: "not-a-real-session", url: baseURL },
    ]);
    const outsideRequests = trackOutsideRequests(page);

    await page.goto("/organizations");

    await expect(page).toHaveURL(LOGIN_WITH_NEXT);
    await expect(
      page.getByRole("heading", { name: "Pilih usaha" }),
    ).not.toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "rejected-session");
    expect(outsideRequests).toEqual([]);
  });
});
