import { describe, expect, it, vi } from "vitest";

import {
  canGoBackTo,
  nextTrail,
  pathOf,
  recordPathname,
  waitForPathname,
} from "./utils";

const LIST = "/o/7/tenants";
const DETAIL = "/o/7/tenants/42";

describe("pathOf", () => {
  it.each([
    [LIST, LIST],
    [`${LIST}?q=rina&page=2`, LIST],
    [`${LIST}#top`, LIST],
    [`${LIST}?q=a#b`, LIST],
  ])("%s → %s", (href, path) => {
    expect(pathOf(href)).toBe(path);
  });
});

describe("canGoBackTo", () => {
  it("goes back when the page before is the one the link leads to", () => {
    expect(canGoBackTo(LIST, LIST)).toBe(true);
  });

  it("ignores the query of the link: going back keeps the list's own search and page", () => {
    expect(canGoBackTo(`${LIST}?page=1`, LIST)).toBe(true);
  });

  it("opens the page when another page came before, or none (a reload, a shared link)", () => {
    expect(canGoBackTo(LIST, "/o/7")).toBe(false);
    expect(canGoBackTo(LIST, null)).toBe(false);
  });
});

describe("nextTrail", () => {
  it("moves the current path to previous", () => {
    expect(nextTrail({ previous: null, current: LIST }, DETAIL)).toEqual({
      previous: LIST,
      current: DETAIL,
    });
  });

  it("keeps the trail when the path did not change (a search or a page of the list)", () => {
    const trail = { previous: "/o/7", current: LIST };
    expect(nextTrail(trail, LIST)).toBe(trail);
  });

  it("starts from nothing", () => {
    expect(nextTrail({ previous: null, current: null }, LIST)).toEqual({
      previous: null,
      current: LIST,
    });
  });
});

describe("waitForPathname", () => {
  it("resolves when the page is shown", async () => {
    vi.useFakeTimers();
    let shown = false;
    const wait = waitForPathname(LIST, 1000).then(() => {
      shown = true;
    });
    recordPathname(DETAIL);
    await Promise.resolve();
    expect(shown).toBe(false);
    recordPathname(LIST);
    await wait;
    expect(shown).toBe(true);
    vi.useRealTimers();
  });

  it("gives up after the timeout", async () => {
    vi.useFakeTimers();
    let shown = false;
    const wait = waitForPathname("/o/7/never", 1000).then(() => {
      shown = true;
    });
    vi.advanceTimersByTime(999);
    await Promise.resolve();
    expect(shown).toBe(false);
    vi.advanceTimersByTime(1);
    await wait;
    expect(shown).toBe(true);
    vi.useRealTimers();
  });
});
