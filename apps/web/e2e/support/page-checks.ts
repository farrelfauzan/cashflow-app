import type { Locator, Page, TestInfo } from "@playwright/test";

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
  await page.screenshot({ path, animations: "disabled" });
  await testInfo.attach(name, { path, contentType: "image/png" });
}

// Waits until the page's finite animations have ended (a dialog zooming in from 95%, a drawer
// sliding), so a size is measured as it ends up, not mid-animation. Spinners and other endless
// animations are skipped.
export async function animationsSettled(target: Page | Locator): Promise<void> {
  const page = "mainFrame" in target ? target : target.page();
  await page.evaluate(async () => {
    // A CSS animation starts on the next frame: give one that was just triggered time to appear.
    await new Promise((done) =>
      requestAnimationFrame(() => requestAnimationFrame(done)),
    );
    await Promise.all(
      document
        .getAnimations()
        .filter(
          (animation) =>
            animation.effect?.getComputedTiming().iterations !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => undefined)),
    );
  });
}
