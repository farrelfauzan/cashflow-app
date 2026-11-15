import { describe, expect, it } from "vitest";

import {
  PHOTO_KEEP_BYTES,
  PHOTO_MAX_BYTES,
  PHOTO_MAX_SIDE,
  fitWithin,
  isTooLarge,
  jpegName,
  keepsOriginal,
  preparePhoto,
} from "./photo";

describe("fitWithin", () => {
  it.each([
    {
      name: "a portrait phone photo is scaled to 2000 on its long side",
      size: { width: 3024, height: 4032 },
      want: { width: 1500, height: 2000 },
    },
    {
      name: "a landscape photo keeps its shape",
      size: { width: 4000, height: 3000 },
      want: { width: 2000, height: 1500 },
    },
    {
      name: "a tall phone screenshot is scaled by its height",
      size: { width: 1080, height: 2340 },
      want: { width: 923, height: 2000 },
    },
    {
      name: "exactly the limit stays as it is",
      size: { width: 2000, height: 1000 },
      want: { width: 2000, height: 1000 },
    },
    {
      name: "a small screenshot is never scaled up",
      size: { width: 480, height: 720 },
      want: { width: 480, height: 720 },
    },
    {
      name: "a very thin image keeps at least one pixel",
      size: { width: 10_000, height: 1 },
      want: { width: 2000, height: 1 },
    },
  ])("$name", ({ size, want }) => {
    expect(fitWithin(size, PHOTO_MAX_SIDE)).toEqual(want);
  });
});

describe("keepsOriginal", () => {
  const small = { width: 1000, height: 1500 };
  it.each([
    { name: "a small JPEG", type: "image/jpeg", bytes: 300_000, want: true },
    { name: "a small PNG", type: "image/png", bytes: 500_000, want: true },
    { name: "a small WebP", type: "image/webp", bytes: 1, want: true },
    {
      name: "a HEIC photo, which the API does not take",
      type: "image/heic",
      bytes: 300_000,
      want: false,
    },
    {
      name: "a JPEG above the keep size",
      type: "image/jpeg",
      bytes: PHOTO_KEEP_BYTES + 1,
      want: false,
    },
    {
      name: "a JPEG at the keep size",
      type: "image/jpeg",
      bytes: PHOTO_KEEP_BYTES,
      want: true,
    },
  ])("$name: $want", ({ type, bytes, want }) => {
    expect(keepsOriginal({ type, bytes, size: small })).toBe(want);
  });

  it("re-encodes a small file that is larger than the longest side", () => {
    expect(
      keepsOriginal({
        type: "image/jpeg",
        bytes: 100_000,
        size: { width: 2001, height: 10 },
      }),
    ).toBe(false);
  });
});

describe("jpegName", () => {
  it.each([
    { name: "IMG_0012.HEIC", want: "IMG_0012.jpg" },
    { name: "bukti.png", want: "bukti.jpg" },
    { name: "screenshot.2026.11.16.webp", want: "screenshot.2026.11.16.jpg" },
    { name: "foto tanpa titik", want: "foto tanpa titik.jpg" },
    { name: "", want: "foto.jpg" },
    { name: ".heic", want: "foto.jpg" },
  ])("$name gives $want", ({ name, want }) => {
    expect(jpegName(name)).toBe(want);
  });
});

describe("isTooLarge", () => {
  it("allows exactly 10 MiB, the API's limit, and refuses a byte more", () => {
    expect(PHOTO_MAX_BYTES).toBe(10 * 1024 * 1024);
    expect(isTooLarge(PHOTO_MAX_BYTES)).toBe(false);
    expect(isTooLarge(PHOTO_MAX_BYTES + 1)).toBe(true);
  });
});

describe("preparePhoto", () => {
  it("sends the file as it is when this runtime cannot decode images", async () => {
    // Node has no createImageBitmap, like a browser that cannot read the file.
    const file = new File(["bukan foto"], "bukti.jpg", { type: "image/jpeg" });
    await expect(preparePhoto(file)).resolves.toBe(file);
  });
});
