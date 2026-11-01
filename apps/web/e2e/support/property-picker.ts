import type { Locator, Page } from "@playwright/test";

// The searchable "Properti" picker of "Sewakan unit". It lists only the first page of properties,
// so a property is found by typing its name, whatever the number of properties in the
// organization.
export async function chooseProperty(
  page: Page,
  scope: Locator,
  name: string,
): Promise<void> {
  await scope.getByRole("combobox", { name: "Properti" }).click();
  await page.getByRole("combobox", { name: "Cari nama properti" }).fill(name);
  await page.getByRole("option", { name, exact: true }).click();
}
