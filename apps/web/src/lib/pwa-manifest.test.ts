import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { describe, expect, it } from "vitest";

import { APP_NAME } from "./app-name";

// public/manifest.webmanifest is a static file (an app/manifest.ts would link it from every
// page, tenant pages included); this keeps it in step with the app.
const PUBLIC_DIR = join(__dirname, "..", "..", "public");

type Manifest = {
  name: string;
  short_name: string;
  start_url: string;
  theme_color: string;
  background_color: string;
  icons: { src: string; sizes: string; purpose?: string }[];
};

const manifest = JSON.parse(
  readFileSync(join(PUBLIC_DIR, "manifest.webmanifest"), "utf8"),
) as Manifest;

describe("PWA manifest", () => {
  it("names the app with APP_NAME", () => {
    expect(manifest.name).toBe(APP_NAME);
    expect(manifest.short_name).toBe(APP_NAME);
  });

  it("opens on the organization picker, a staff route", () => {
    expect(manifest.start_url).toBe("/organizations");
  });

  it("uses the palette's cream", () => {
    expect(manifest.theme_color).toBe("#f5f3ee");
    expect(manifest.background_color).toBe("#f5f3ee");
  });

  it("lists icons that exist, including a maskable one", () => {
    for (const icon of manifest.icons) {
      expect(existsSync(join(PUBLIC_DIR, icon.src))).toBe(true);
    }
    expect(manifest.icons.some((icon) => icon.purpose === "maskable")).toBe(
      true,
    );
  });
});
