import { readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { APP_NAME } from "@/lib/app-name";

import {
  MARK_ARC_PATH,
  MARK_ARROWHEAD_PATH,
  MARK_VIEW_BOX,
  WORDMARK_LEAD,
  WORDMARK_TAIL,
} from "./utils";

const ICON_SVG = readFileSync(
  join(__dirname, "..", "..", "..", "..", "public", "icon.svg"),
  "utf8",
);

describe("the ReCash mark", () => {
  it("is the one public/icon.svg draws", () => {
    expect(ICON_SVG).toContain(`viewBox="${MARK_VIEW_BOX}"`);
    expect(ICON_SVG).toContain(`d="${MARK_ARC_PATH}"`);
    expect(ICON_SVG).toContain(`d="${MARK_ARROWHEAD_PATH}"`);
  });

  it("has the approved colors in public/icon.svg", () => {
    for (const color of ["#1D6B63", "#F5F3EE", "#F5B82E", "#C7870B"]) {
      expect(ICON_SVG).toContain(color);
    }
  });
});

describe("the wordmark", () => {
  it("reads as the app's name", () => {
    expect(WORDMARK_LEAD).toBe("Re");
    expect(WORDMARK_TAIL).toBe("Cash");
    expect(`${WORDMARK_LEAD}${WORDMARK_TAIL}`).toBe(APP_NAME);
  });
});
