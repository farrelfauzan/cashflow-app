import type { Page, TestInfo } from "@playwright/test";

// Collects every request the page makes to a host other than localhost. The app self-hosts its
// font and talks only to its own /api, so the list must stay empty.
export function trackOutsideRequests(page: Page): string[] {
  const outside: string[] = [];
  page.on("request", (request) => {
    const { hostname } = new URL(request.url());
    if (hostname !== "localhost") outside.push(request.url());
  });
  return outside;
}

// A full-page screenshot attached to the report, one per project (ipad, phone).
export async function attachFullPageScreenshot(
  page: Page,
  testInfo: TestInfo,
  name: string,
): Promise<void> {
  const path = testInfo.outputPath(`${name}-${testInfo.project.name}.png`);
  await page.screenshot({ path, fullPage: true });
  await testInfo.attach(name, { path, contentType: "image/png" });
}

// What the screen shows, one per project: for an open side panel, which is fixed to the viewport,
// so a full-page capture of a scrolled page would draw it part way down.
export async function attachViewportScreenshot(
  page: Page,
  testInfo: TestInfo,
  name: string,
): Promise<void> {
  const path = testInfo.outputPath(`${name}-${testInfo.project.name}.png`);
  await page.screenshot({ path });
  await testInfo.attach(name, { path, contentType: "image/png" });
}
