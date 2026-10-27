import { expect, test } from "@playwright/test";

// Minimum body text size per project, CLAUDE.md "UX rules".
const MIN_BODY_FONT_PX: Record<string, number> = { ipad: 16, phone: 14 };

test("placeholder page shows the app name in Indonesian at a readable size", async ({
  page,
}, testInfo) => {
  const minFontPx = MIN_BODY_FONT_PX[testInfo.project.name];
  if (minFontPx === undefined) {
    throw new Error(`No font-size rule for project ${testInfo.project.name}`);
  }

  // The font is self-hosted, so the page must not reach any other host.
  const outsideRequests: string[] = [];
  page.on("request", (request) => {
    const { hostname } = new URL(request.url());
    if (hostname !== "localhost") {
      outsideRequests.push(request.url());
    }
  });

  await page.goto("/");

  await expect(page).toHaveTitle("Pengelolaan Sewa");
  await expect(page.locator("html")).toHaveAttribute("lang", "id");
  await expect(
    page.getByRole("heading", { level: 1, name: "Pengelolaan Sewa" }),
  ).toBeVisible();
  await expect(page.getByText("Aplikasi sedang disiapkan.")).toBeVisible();

  const fontSizes = await page.evaluate(() => {
    const px = (el: Element) => parseFloat(getComputedStyle(el).fontSize);
    const paragraph = document.querySelector("main p");
    return {
      body: px(document.body),
      paragraph: paragraph ? px(paragraph) : 0,
    };
  });
  expect(fontSizes.body).toBeGreaterThanOrEqual(minFontPx);
  expect(fontSizes.paragraph).toBeGreaterThanOrEqual(minFontPx);

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

  const screenshot = testInfo.outputPath(
    `placeholder-${testInfo.project.name}.png`,
  );
  await page.screenshot({ path: screenshot, fullPage: true });
  await testInfo.attach("placeholder", {
    path: screenshot,
    contentType: "image/png",
  });

  expect(outsideRequests).toEqual([]);
});
