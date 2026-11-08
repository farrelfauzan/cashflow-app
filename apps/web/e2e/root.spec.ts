import { expect, test } from "@playwright/test";

import { APP_NAME } from "../src/lib/app-name";
import {
  attachFullPageScreenshot,
  trackOutsideRequests,
} from "./support/page-checks";
import { HENDRA } from "./support/staff";

// "/" has no page of its own: src/proxy.ts sends it to the organization list, or to login
// without a session.

test.describe("signed out", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("opens login in Indonesian with the self-hosted font", async ({
    page,
  }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);

    await page.goto("/");

    await expect(page).toHaveURL("/login");
    await expect(page).toHaveTitle(APP_NAME);
    await expect(page.locator("html")).toHaveAttribute("lang", "id");
    await expect(
      page.getByRole("heading", { level: 1, name: "Masuk" }),
    ).toBeVisible();

    // The heading renders in Plus Jakarta Sans, loaded from this app (not the fallback).
    const font = await page.evaluate(async () => {
      await document.fonts.ready;
      const heading = document.querySelector("h1");
      return {
        family: heading ? getComputedStyle(heading).fontFamily : "",
        loaded: [...document.fonts].some(
          (face) =>
            face.family.replace(/["']/g, "") === "Plus Jakarta Sans" &&
            face.status === "loaded",
        ),
      };
    });
    expect(font.family).toMatch(/^["']?Plus Jakarta Sans["']?,/);
    expect(font.loaded).toBe(true);

    // The ReCash logo (board Logo-Opsi-3): the inline mark with its coin, the wordmark, and the
    // same mark as the tab's icon; the manifest's icons are all served.
    await expect(page.getByText(APP_NAME, { exact: true })).toBeVisible();
    const coin = page.locator("svg circle.fill-coin");
    await expect(coin).toHaveCount(1);
    await expect(coin).toHaveCSS("fill", "rgb(245, 184, 46)");
    await expect(page.locator('link[rel="icon"]')).toHaveAttribute(
      "href",
      /^\/icon\.svg/,
    );
    for (const icon of [
      "/icon.svg",
      "/icon-192.png",
      "/icon-512.png",
      "/icon-maskable-512.png",
      "/apple-touch-icon.png",
    ]) {
      expect((await page.request.get(icon)).status()).toBe(200);
    }

    await attachFullPageScreenshot(page, testInfo, "root-signed-out");
    expect(outsideRequests).toEqual([]);
  });
});

test.describe("signed in", () => {
  // Saved by e2e/auth.setup.ts through the real login code flow.
  test.use({ storageState: HENDRA.storageState });

  test("opens the organization list", async ({ page }, testInfo) => {
    const outsideRequests = trackOutsideRequests(page);

    await page.goto("/");

    await expect(page).toHaveURL("/organizations");
    await expect(
      page.getByRole("heading", { level: 1, name: "Pilih usaha" }),
    ).toBeVisible();

    await attachFullPageScreenshot(page, testInfo, "root-signed-in");
    expect(outsideRequests).toEqual([]);
  });
});
