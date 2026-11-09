import { describe, expect, it } from "vitest";

import {
  createDirtyRegistry,
  swipeOffset,
  swipeReleaseAction,
  swipeVelocity,
} from "./utils";

describe("swipeReleaseAction", () => {
  // A 780px sheet (390×844 phone at 92dvh, rounded): a quarter is 195px.
  const sheetHeight = 780;

  it.each([
    { name: "not moved", distance: 0, velocity: 0, want: "snap" },
    { name: "10% held still", distance: 78, velocity: 0, want: "snap" },
    { name: "exactly a quarter", distance: 195, velocity: 0, want: "snap" },
    { name: "past a quarter", distance: 196, velocity: 0, want: "close" },
    { name: "40% held still", distance: 312, velocity: 0, want: "close" },
    { name: "40% moving up", distance: 312, velocity: -1, want: "close" },
    { name: "short flick down", distance: 40, velocity: 0.8, want: "close" },
    {
      name: "short move at the flick speed exactly",
      distance: 40,
      velocity: 0.5,
      want: "snap",
    },
    {
      name: "short move going back up",
      distance: 40,
      velocity: -0.8,
      want: "snap",
    },
    {
      name: "fast jitter of a tap is not a flick",
      distance: 4,
      velocity: 2,
      want: "snap",
    },
    { name: "flick of the minimum", distance: 8, velocity: 0.6, want: "close" },
  ])("$name", ({ distance, velocity, want }) => {
    expect(swipeReleaseAction({ distance, sheetHeight, velocity })).toBe(want);
  });

  it("snaps back when the sheet has no height", () => {
    expect(
      swipeReleaseAction({ distance: 100, sheetHeight: 0, velocity: 5 }),
    ).toBe("snap");
  });
});

describe("swipeVelocity", () => {
  it.each([
    {
      name: "no samples",
      samples: [],
      release: { y: 300, time: 1000 },
      want: 0,
    },
    {
      name: "stopped longer than the window before release",
      samples: [
        { y: 100, time: 700 },
        { y: 300, time: 850 },
      ],
      release: { y: 300, time: 1000 },
      want: 0,
    },
    {
      name: "steady move down within the window",
      samples: [
        { y: 100, time: 800 },
        { y: 200, time: 920 },
        { y: 240, time: 960 },
      ],
      release: { y: 280, time: 1000 },
      // From the oldest sample in the last 100 ms (920) to the release.
      want: 1,
    },
    {
      name: "moving up",
      samples: [{ y: 300, time: 950 }],
      release: { y: 250, time: 1000 },
      want: -1,
    },
    {
      name: "a sample at the release time only",
      samples: [{ y: 300, time: 1000 }],
      release: { y: 300, time: 1000 },
      want: 0,
    },
  ])("$name", ({ samples, release, want }) => {
    expect(swipeVelocity(samples, release)).toBeCloseTo(want);
  });
});

describe("swipeOffset", () => {
  it.each([
    { name: "down", start: 100, current: 160, want: 60 },
    { name: "not moved", start: 100, current: 100, want: 0 },
    { name: "up stays at rest", start: 100, current: 40, want: 0 },
  ])("$name", ({ start, current, want }) => {
    expect(swipeOffset(start, current)).toBe(want);
  });
});

describe("createDirtyRegistry", () => {
  it("is dirty while any form reports unsaved changes", () => {
    const registry = createDirtyRegistry();
    expect(registry.isDirty()).toBe(false);
    registry.report("a", true);
    registry.report("b", true);
    expect(registry.isDirty()).toBe(true);
    registry.report("a", false);
    expect(registry.isDirty()).toBe(true);
    registry.report("b", false);
    expect(registry.isDirty()).toBe(false);
  });

  it("forgets a form that reports clean twice, as an unmount after a reset does", () => {
    const registry = createDirtyRegistry();
    registry.report("a", true);
    registry.report("a", false);
    registry.report("a", false);
    expect(registry.isDirty()).toBe(false);
  });
});
