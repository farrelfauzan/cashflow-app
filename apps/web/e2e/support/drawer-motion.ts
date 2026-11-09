import { expect, type Page } from "@playwright/test";

import { ORG_HOME_URL } from "./shell";

// Helpers for the side drawer's motion spec: reaching the properties page, and recording which
// CSS animations the drawer runs.

// One animation the page ran: the data-slot of the element and the keyframes' name. The drawer's
// panel is "sheet-content" and its backdrop "sheet-overlay"; tw-animate-css names its keyframes
// "enter" and "exit".
export type RecordedAnimation = { slot: string; name: string };

declare global {
  interface Window {
    __recordedAnimations?: RecordedAnimation[];
  }
}

// Opens the properties page of an organization, by name when the member has several.
export async function openPropertiesPage(
  page: Page,
  organizations: readonly string[],
  name: string,
): Promise<void> {
  await page.goto("/organizations");
  if (organizations.length > 1) {
    await page
      .getByRole("list", { name: "Daftar usaha" })
      .getByRole("link", { name })
      .click();
  }
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/properties`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Properti" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar properti" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
}

// Starts recording every animationstart on the page (capturing, so it sees the portal's
// elements). Call it after the page has loaded and before the action that should animate.
export async function recordAnimations(page: Page): Promise<void> {
  await page.evaluate(() => {
    window.__recordedAnimations = [];
    document.addEventListener(
      "animationstart",
      (event) => {
        const target = event.target;
        const slot =
          target instanceof HTMLElement ? (target.dataset.slot ?? "") : "";
        window.__recordedAnimations?.push({ slot, name: event.animationName });
      },
      true,
    );
  });
}

// What was recorded so far.
export async function recordedAnimations(
  page: Page,
): Promise<RecordedAnimation[]> {
  return page.evaluate(() => window.__recordedAnimations ?? []);
}

// Forgets what was recorded, so the next check sees only what follows.
export async function clearRecordedAnimations(page: Page): Promise<void> {
  await page.evaluate(() => {
    window.__recordedAnimations = [];
  });
}
