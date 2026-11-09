import { expect, type Locator, type Page } from "@playwright/test";

import type { ToastTone } from "../../src/components/client/toast/type";

// The toaster (board "Notifikasi singkat"): Sonner's region, labelled "Notifikasi alt+T", mounted
// once on staff pages and never on tenant pages. Being aria-live, it stays in the accessibility
// tree while a sheet or dialog is open.
export function toastRegion(page: Page): Locator {
  return page.getByRole("region", { name: /Notifikasi/ });
}

// One toast by its title.
export function toast(page: Page, title: string): Locator {
  return toastRegion(page)
    .locator("[data-tone]")
    .filter({ has: page.getByText(title, { exact: true }) });
}

// The toast shows with its color by meaning and, when given, its second line.
export async function expectToast(
  page: Page,
  title: string,
  tone: ToastTone,
  description?: string,
): Promise<Locator> {
  const shown = toast(page, title);
  await expect(shown).toBeVisible();
  await expect(shown).toHaveAttribute("data-tone", tone);
  if (description !== undefined) await expect(shown).toContainText(description);
  return shown;
}
