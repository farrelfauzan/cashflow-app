import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { useHydrated } from "./use-hydrated";

function Probe() {
  return createElement("span", null, useHydrated() ? "hydrated" : "server");
}

// The browser side (false while hydrating, true after it and on client navigations) is covered
// by e2e/dev-smoke.spec.ts, which needs a real hydration.
describe("useHydrated", () => {
  it("is false in a server render", () => {
    expect(renderToString(createElement(Probe))).toBe("<span>server</span>");
  });
});
