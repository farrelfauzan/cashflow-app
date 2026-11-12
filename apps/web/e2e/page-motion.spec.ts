import {
  expect,
  test,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";

import { isPhone, ORG_HOME_URL } from "./support/shell";
import { HENDRA } from "./support/staff";

// The push and pop between pages on the phone (board HP-Pindah) and the pressed rows and tabs
// (board HP-Tekan), against the real API (seed-demo data). Nothing is changed.
//
// Every view transition the page starts is recorded with its types and, once it is ready, the
// animation each of its layers runs, so the tests
// see which slide played, on which layer, for how long, that the top bar and the tab bar stay
// still, and that iPad and desktop play none.

const GRIYA = "Griya Sejahtera";
const RINA = "Rina Wulandari";

type Recorded = {
  types: string[];
  skipped: boolean;
  // "::view-transition-new(root)" → "page-push-in 280ms"
  layers: Record<string, string>;
};

const LAYERS = [
  "::view-transition-old(root)",
  "::view-transition-new(root)",
  "::view-transition-group(phone-top-bar)",
  "::view-transition-new(phone-top-bar)",
  "::view-transition-group(phone-tab-bar)",
  "::view-transition-new(phone-tab-bar)",
];

declare global {
  interface Window {
    __transitions: Recorded[];
  }
}

test.use({ storageState: HENDRA.storageState });

test.beforeEach(async ({ page }) => {
  await page.addInitScript((layers_: string[]) => {
    window.__transitions = [];
    const start = document.startViewTransition?.bind(document);
    if (start === undefined) return;
    document.startViewTransition = ((
      arg?: ViewTransitionUpdateCallback | StartViewTransitionOptions,
    ) => {
      const transition = start(arg as StartViewTransitionOptions);
      const types =
        typeof arg === "object" && arg !== null && arg.types
          ? [...arg.types]
          : [];
      transition.ready.then(
        () => {
          // The animations that run, not the computed style: a layer can name an animation in
          // CSS and still not exist (React takes the screen out of a transition it thinks the
          // screen is not part of).
          const layers: Record<string, string> = {};
          for (const animation of document.documentElement.getAnimations({
            subtree: true,
          })) {
            const effect = animation.effect as KeyframeEffect | null;
            const pseudo = effect?.pseudoElement ?? "";
            const name = (animation as CSSAnimation).animationName ?? "";
            if (!layers_.includes(pseudo) || name === "") continue;
            layers[pseudo] =
              `${name} ${Number(effect?.getTiming().duration)}ms`;
          }
          window.__transitions.push({ types, skipped: false, layers });
        },
        () => window.__transitions.push({ types, skipped: true, layers: {} }),
      );
      return transition;
    }) as typeof document.startViewTransition;
  }, LAYERS);
});

async function openTenants(page: Page) {
  await page.goto("/organizations");
  await page
    .getByRole("list", { name: "Daftar usaha" })
    .getByRole("link", { name: GRIYA })
    .click();
  await expect(page).toHaveURL(ORG_HOME_URL);
  await page.goto(`${page.url()}/tenants`);
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
  const list = page.getByRole("list", { name: "Daftar penyewa" });
  await expect(list.getByRole("listitem").first()).toBeVisible();
  return list;
}

async function transitions(page: Page): Promise<Recorded[]> {
  return page.evaluate(() => window.__transitions);
}

async function lastTransition(page: Page): Promise<Recorded> {
  await expect
    .poll(async () => (await transitions(page)).length)
    .toBeGreaterThan(0);
  const last = (await transitions(page)).at(-1);
  if (last === undefined) throw new Error("no view transition was recorded");
  return last;
}

// The screen's own layers that animate: "<layer> <animation> <duration>".
function rootAnimations(recorded: Recorded) {
  return Object.entries(recorded.layers)
    .filter(([layer]) => layer.endsWith("(root)"))
    .map(([layer, value]) => `${layer} ${value}`)
    .sort();
}

// The top bar and the tab bar stay still.
function otherAnimations(recorded: Recorded) {
  return Object.entries(recorded.layers).filter(
    ([layer, value]) => !layer.endsWith("(root)") && !value.startsWith("none"),
  );
}

async function clearTransitions(page: Page) {
  await page.evaluate(() => {
    window.__transitions = [];
  });
}

async function expectTenantPage(page: Page) {
  await expect(page).toHaveURL(/\/tenants\/[0-9a-f-]{36}$/);
  await expect(
    page.getByRole("heading", { level: 1, name: RINA }),
  ).toBeVisible();
}

function backLink(page: Page) {
  return page.getByRole("main").getByRole("link", { name: "Penyewa" });
}

test("a tenant opens with a push and ‹ Penyewa returns with a pop, to the list as it was left", async ({
  page,
}, testInfo: TestInfo) => {
  const list = await openTenants(page);

  // A search, so the way back can show it kept the list as it was.
  const search = page.getByLabel("Cari nama, nomor, atau unit");
  await search.fill("Rina");
  await expect(page).toHaveURL(/[?&]q=Rina/);
  // Other specs add tenants of their own; every row left is one that matches.
  await expect(
    list.getByRole("listitem").filter({ hasNotText: "Rina" }),
  ).toHaveCount(0);
  const listUrl = page.url();
  await clearTransitions(page);

  await list
    .getByRole("listitem")
    .filter({ hasText: RINA })
    .getByRole("link")
    .click();
  await expectTenantPage(page);
  const push = await lastTransition(page);
  expect(push.types).toEqual(["nav-forward"]);
  expect(push.skipped).toBe(false);
  expect(otherAnimations(push)).toEqual([]);
  if (isPhone(testInfo)) {
    expect(rootAnimations(push)).toEqual([
      "::view-transition-new(root) page-push-in 280ms",
      "::view-transition-old(root) page-push-out 280ms",
    ]);
  } else {
    expect(rootAnimations(push)).toEqual([]);
  }

  await clearTransitions(page);
  await backLink(page).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
  // Going back in the history: the ‹ link starts this transition itself, so it has no types; the
  // direction reaches the CSS through data-page-motion.
  const pop = await lastTransition(page);
  expect(pop.types).toEqual([]);
  expect(pop.skipped).toBe(false);
  expect(otherAnimations(pop)).toEqual([]);
  if (isPhone(testInfo)) {
    expect(rootAnimations(pop)).toEqual([
      "::view-transition-new(root) page-pop-in 240ms",
      "::view-transition-old(root) page-pop-out 240ms",
    ]);
  } else {
    expect(rootAnimations(pop)).toEqual([]);
  }

  // It went back in the history: the list with its search, and no new entry to step through (the
  // search replaced the list's own entry, so Back from it is Beranda).
  expect(page.url()).toBe(listUrl);
  await expect(search).toHaveValue("Rina");
  await expect(page.locator("html")).not.toHaveAttribute("data-page-motion");
  await page.goBack();
  await expect(page).toHaveURL(ORG_HOME_URL);
});

test("‹ Penyewa returns to the list scrolled where it was left, on the phone", async ({
  page,
}, testInfo) => {
  test.skip(!isPhone(testInfo), "The phone's list is long enough to scroll.");
  const list = await openTenants(page);
  const row = list.getByRole("listitem").filter({ hasText: RINA });
  await row.evaluate((el) => el.scrollIntoView({ block: "center" }));
  const scrolled = await page.evaluate(() => window.scrollY);
  expect(scrolled).toBeGreaterThan(100);

  await row.getByRole("link").click();
  await expectTenantPage(page);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
  await backLink(page).click();
  await expect(row).toBeVisible();
  await expect
    .poll(() => page.evaluate(() => window.scrollY))
    .toBeGreaterThanOrEqual(scrolled - 2);
  expect(await page.evaluate(() => window.scrollY)).toBeLessThanOrEqual(
    scrolled + 2,
  );
});

test("the browser's Back changes the page at once", async ({ page }) => {
  const list = await openTenants(page);
  await list
    .getByRole("listitem")
    .filter({ hasText: RINA })
    .getByRole("link")
    .click();
  await expectTenantPage(page);
  await clearTransitions(page);
  await page.goBack();
  await expect(
    page.getByRole("heading", { level: 1, name: "Penyewa" }),
  ).toBeVisible();
  expect(await transitions(page)).toEqual([]);
});

test("‹ Penyewa on a tenant opened directly opens the list, with a pop", async ({
  page,
}, testInfo) => {
  const list = await openTenants(page);
  await list
    .getByRole("listitem")
    .filter({ hasText: RINA })
    .getByRole("link")
    .click();
  await expectTenantPage(page);
  const tenantUrl = page.url();

  // A reload forgets the page before; ‹ opens the list instead of leaving the app.
  await page.reload();
  await expectTenantPage(page);
  await backLink(page).click();
  await expect(page).toHaveURL(/\/tenants$/);
  const pop = await lastTransition(page);
  expect(pop.types).toEqual(["nav-back"]);
  expect(rootAnimations(pop).length).toBe(isPhone(testInfo) ? 2 : 0);

  // That was a new page, so Back returns to the tenant.
  await page.goBack();
  await expect(page).toHaveURL(tenantUrl);
});

test("the tab bar and the list's search change the page at once", async ({
  page,
}, testInfo) => {
  test.skip(!isPhone(testInfo), "The tab bar is on the phone only.");
  await openTenants(page);
  await clearTransitions(page);

  await page
    .getByRole("navigation", { name: "Menu utama" })
    .getByRole("link", { name: "Properti" })
    .click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Properti" }),
  ).toBeVisible();
  await page
    .getByRole("navigation", { name: "Menu utama" })
    .getByRole("link", { name: "Penyewa" })
    .click();
  await page.getByLabel("Cari nama, nomor, atau unit").fill("Budi");
  await expect(page).toHaveURL(/[?&]q=Budi/);

  expect(await transitions(page)).toEqual([]);
});

test.describe("with reduced motion", () => {
  test.use({ reducedMotion: "reduce" });

  test("the new page fades in without sliding", async ({ page }, testInfo) => {
    test.skip(!isPhone(testInfo), "Pages slide on the phone only.");
    const list = await openTenants(page);
    await clearTransitions(page);
    await list
      .getByRole("listitem")
      .filter({ hasText: RINA })
      .getByRole("link")
      .click();
    await expectTenantPage(page);
    expect(rootAnimations(await lastTransition(page))).toEqual([
      "::view-transition-new(root) page-fade-in 150ms",
    ]);
  });
});

// Holds :active on an element, as a finger on it would, through Chromium's DevTools protocol:
// headless Chromium does not set :active from emulated touch. What the test proves is the pressed
// style and where it applies; the browser's own touch handling is not under test.
async function holdActive(page: Page, locator: Locator) {
  await locator.evaluate((el) => el.setAttribute("data-e2e-pressed", ""));
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("DOM.enable");
  await cdp.send("CSS.enable");
  const { root } = await cdp.send("DOM.getDocument");
  const { nodeId } = await cdp.send("DOM.querySelector", {
    nodeId: root.nodeId,
    selector: "[data-e2e-pressed]",
  });
  await cdp.send("CSS.forcePseudoState", {
    nodeId,
    forcedPseudoClasses: ["active"],
  });
  return async () => {
    await cdp.send("CSS.forcePseudoState", { nodeId, forcedPseudoClasses: [] });
    await cdp.detach();
    await locator.evaluate((el) => el.removeAttribute("data-e2e-pressed"));
  };
}

test("a row and a tab look pressed while held, on the phone", async ({
  page,
  browserName,
}, testInfo) => {
  test.skip(
    !isPhone(testInfo),
    "Pressed rows are the phone's (board HP-Tekan).",
  );
  test.skip(
    browserName !== "chromium",
    ":active is held through Chromium's DevTools protocol.",
  );
  const list = await openTenants(page);
  const row = list
    .getByRole("listitem")
    .filter({ hasText: RINA })
    .getByRole("link");
  const look = () =>
    row.evaluate((el) => {
      const style = getComputedStyle(el);
      return `${style.backgroundColor} ${style.scale}`;
    });
  const idleRow = await look();
  let lift = await holdActive(page, row);
  await expect.poll(look).toMatch(/ 0\.985$/);
  expect(await look()).not.toBe(idleRow.replace(/ \S+$/, " 0.985"));
  await lift();
  await expect.poll(look).toBe(idleRow);

  const tab = page
    .getByRole("navigation", { name: "Menu utama" })
    .getByRole("link", { name: "Properti" });
  const pill = tab.locator("span").first();
  const pillBackground = () =>
    pill.evaluate((el) => getComputedStyle(el).backgroundColor);
  const idlePill = await pillBackground();
  lift = await holdActive(page, tab);
  await expect.poll(pillBackground).not.toBe(idlePill);
  await lift();
  await expect.poll(pillBackground).toBe(idlePill);
});
