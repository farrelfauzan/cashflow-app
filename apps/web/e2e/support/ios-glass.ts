import type { CDPSession, Locator, Page } from "@playwright/test";

// The iOS glass (T-097e, board iOS-Aturan) is gated in CSS on -webkit-touch-callout, which only
// WebKit on iOS and iPadOS knows, so Chromium never matches it. These helpers let the spec see
// both sides: Chromium as it is (Android and desktop) and Chromium with the gate opened.

const IOS_GATE = "touch-callout";

// Opens the iOS gate in the page's stylesheets: every @supports rule whose condition names
// -webkit-touch-callout is replaced, in place, by the rules inside it. In place keeps each rule in
// its cascade layer, so the page cascades as it would on an iPhone. Returns how many it opened.
export async function openIosGate(page: Page): Promise<number> {
  return page.evaluate((gate) => {
    let opened = 0;
    const unwrap = (parent: CSSStyleSheet | CSSGroupingRule) => {
      for (let index = parent.cssRules.length - 1; index >= 0; index -= 1) {
        const rule = parent.cssRules[index];
        if (
          rule instanceof CSSSupportsRule &&
          rule.conditionText.includes(gate)
        ) {
          const inner = Array.from(rule.cssRules, (child) => child.cssText);
          parent.deleteRule(index);
          for (const text of inner.reverse()) parent.insertRule(text, index);
          opened += 1;
        } else if (rule instanceof CSSGroupingRule) {
          unwrap(rule);
        }
      }
    };
    for (const sheet of Array.from(document.styleSheets)) unwrap(sheet);
    return opened;
  }, IOS_GATE);
}

// The look a glass element has, as computed.
export type GlassLook = {
  backdropFilter: string;
  backgroundColor: string;
  borderRadius: string;
  borderTopColor: string;
};

export async function glassLook(element: Locator): Promise<GlassLook> {
  return element.evaluate((el) => {
    const style = getComputedStyle(el);
    return {
      backdropFilter: style.backdropFilter,
      backgroundColor: style.backgroundColor,
      borderRadius: style.borderTopLeftRadius,
      borderTopColor: style.borderTopColor,
    };
  });
}

// Turns Reduce Transparency on through the DevTools protocol (Playwright's emulateMedia has no
// option for it). Safari does not expose it to CSS today; Chromium does, so this proves the rule.
export async function reduceTransparency(page: Page): Promise<CDPSession> {
  const session = await page.context().newCDPSession(page);
  await session.send("Emulation.setEmulatedMedia", {
    features: [{ name: "prefers-reduced-transparency", value: "reduce" }],
  });
  return session;
}

// The cream glass of the floating chrome: cream at 80%.
export const GLASS_CREAM = "rgba(245, 243, 238, 0.8)";
export const GLASS_BLUR = "blur(20px) saturate(1.6)";
