import { expect, test } from "@playwright/test";

import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { HENDRA } from "./support/staff";

// /organizations against the real Go API: the session gate in src/proxy.ts, the server-side
// API call that forwards cf_session, and the API's 401 sent back to login.

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

test.describe("signed in", () => {
  // Saved by e2e/auth.setup.ts through the real login code flow.
  test.use({ storageState: HENDRA.storageState });

  test("lists the member's organizations from the API", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);

    await page.goto("/organizations");

    await expect(page).toHaveURL("/organizations");
    await expect(
      page.getByRole("heading", { level: 1, name: "Organisasi" }),
    ).toBeVisible();
    await expect(page.getByText(`Masuk sebagai ${HENDRA.name}`)).toBeVisible();
    // The API orders them by display name.
    await expect(
      page.getByRole("list", { name: "Organisasi" }).getByRole("listitem"),
    ).toHaveText([...HENDRA.organizations]);

    await attachFullPageScreenshot(page, testInfo, "organizations");
    expect(outsideRequests).toEqual([]);
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
      page.getByRole("heading", { name: "Organisasi" }),
    ).not.toBeVisible();
    await attachFullPageScreenshot(page, testInfo, "rejected-session");
    expect(outsideRequests).toEqual([]);
  });
});
