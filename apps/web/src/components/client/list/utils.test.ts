import { describe, expect, it } from "vitest";

import type { PageItem } from "./type";
import {
  clampPage,
  ELLIPSIS,
  PAGE_SIZES,
  pageCount,
  pageItems,
  pageStatusText,
  rangeText,
  showsPageSizeSelect,
  toPageSize,
} from "./utils";

describe("pageCount", () => {
  it.each([
    { name: "an empty list", total: 0, pageSize: 20, want: 1 },
    { name: "fewer rows than a page", total: 7, pageSize: 20, want: 1 },
    { name: "exactly one page", total: 20, pageSize: 20, want: 1 },
    { name: "one row over a page", total: 21, pageSize: 20, want: 2 },
    { name: "a partly filled last page", total: 134, pageSize: 20, want: 7 },
    { name: "the largest page size", total: 250, pageSize: 100, want: 3 },
    { name: "a page size of zero", total: 50, pageSize: 0, want: 1 },
  ])("is $want for $name", ({ total, pageSize, want }) => {
    expect(pageCount(total, pageSize)).toBe(want);
  });
});

describe("clampPage", () => {
  it.each([
    { name: "a page inside", page: 3, count: 7, want: 3 },
    { name: "page zero", page: 0, count: 7, want: 1 },
    { name: "a negative page", page: -4, count: 7, want: 1 },
    { name: "a page past the end", page: 9, count: 7, want: 7 },
    { name: "a fraction", page: 2.8, count: 7, want: 2 },
    { name: "not a number", page: Number.NaN, count: 7, want: 1 },
  ])("gives $want for $name", ({ page, count, want }) => {
    expect(clampPage(page, count)).toBe(want);
  });
});

describe("pageItems", () => {
  const e = ELLIPSIS;

  it.each<{ name: string; page: number; count: number; want: PageItem[] }>([
    { name: "a single page", page: 1, count: 1, want: [1] },
    {
      name: "seven pages shows all",
      page: 4,
      count: 7,
      want: [1, 2, 3, 4, 5, 6, 7],
    },
    { name: "three pages", page: 2, count: 3, want: [1, 2, 3] },
    {
      name: "the first page of 20",
      page: 1,
      count: 20,
      want: [1, 2, 3, 4, 5, e, 20],
    },
    { name: "page 2 of 20", page: 2, count: 20, want: [1, 2, 3, 4, 5, e, 20] },
    { name: "page 3 of 20", page: 3, count: 20, want: [1, 2, 3, 4, 5, e, 20] },
    {
      name: "page 4 of 20, where the gap of one page shows its number",
      page: 4,
      count: 20,
      want: [1, 2, 3, 4, 5, e, 20],
    },
    { name: "page 5 of 20", page: 5, count: 20, want: [1, e, 4, 5, 6, e, 20] },
    {
      name: "page 10 of 20",
      page: 10,
      count: 20,
      want: [1, e, 9, 10, 11, e, 20],
    },
    {
      name: "page 17 of 20, where the gap of one page shows its number",
      page: 17,
      count: 20,
      want: [1, e, 16, 17, 18, 19, 20],
    },
    {
      name: "page 18 of 20",
      page: 18,
      count: 20,
      want: [1, e, 16, 17, 18, 19, 20],
    },
    {
      name: "page 19 of 20",
      page: 19,
      count: 20,
      want: [1, e, 16, 17, 18, 19, 20],
    },
    {
      name: "the last page of 20",
      page: 20,
      count: 20,
      want: [1, e, 16, 17, 18, 19, 20],
    },
    { name: "page 4 of 8", page: 4, count: 8, want: [1, 2, 3, 4, 5, e, 8] },
    { name: "page 5 of 8", page: 5, count: 8, want: [1, e, 4, 5, 6, 7, 8] },
    {
      name: "page 5 of 9, a gap of two on both sides",
      page: 5,
      count: 9,
      want: [1, e, 4, 5, 6, e, 9],
    },
    {
      name: "a page below 1",
      page: -3,
      count: 20,
      want: [1, 2, 3, 4, 5, e, 20],
    },
    {
      name: "a page past the end",
      page: 99,
      count: 20,
      want: [1, e, 16, 17, 18, 19, 20],
    },
  ])("shows $name", ({ page, count, want }) => {
    expect(pageItems(page, count)).toEqual(want);
  });

  it("never puts two ellipses side by side or one for a single page", () => {
    for (let count = 1; count <= 40; count++) {
      for (let page = 1; page <= count; page++) {
        const items = pageItems(page, count);
        items.forEach((item, i) => {
          if (item !== ELLIPSIS) return;
          const before = items[i - 1];
          const after = items[i + 1];
          expect(typeof before).toBe("number");
          expect(typeof after).toBe("number");
          expect((after as number) - (before as number)).toBeGreaterThan(2);
        });
        expect(items).toContain(page);
        expect(items[0]).toBe(1);
        expect(items.at(-1)).toBe(count);
      }
    }
  });
});

describe("rangeText", () => {
  it.each([
    {
      name: "the first page",
      page: 1,
      pageSize: 20,
      total: 134,
      want: "Menampilkan 1–20 dari 134",
    },
    {
      name: "a middle page",
      page: 2,
      pageSize: 20,
      total: 134,
      want: "Menampilkan 21–40 dari 134",
    },
    {
      name: "a short last page",
      page: 7,
      pageSize: 20,
      total: 134,
      want: "Menampilkan 121–134 dari 134",
    },
    {
      name: "fewer rows than a page",
      page: 1,
      pageSize: 20,
      total: 5,
      want: "Menampilkan 1–5 dari 5",
    },
    {
      name: "a single row",
      page: 1,
      pageSize: 20,
      total: 1,
      want: "Menampilkan 1–1 dari 1",
    },
    {
      name: "thousands grouped with dots",
      page: 2,
      pageSize: 100,
      total: 1250,
      want: "Menampilkan 101–200 dari 1.250",
    },
    {
      name: "a page past the end",
      page: 9,
      pageSize: 20,
      total: 134,
      want: "Menampilkan 121–134 dari 134",
    },
  ])("reads $name", ({ page, pageSize, total, want }) => {
    expect(rangeText(page, pageSize, total)).toBe(want);
  });

  it("is null for an empty list", () => {
    expect(rangeText(1, 20, 0)).toBeNull();
  });
});

describe("pageStatusText", () => {
  it("says which page of how many", () => {
    expect(pageStatusText(2, 7)).toBe("Halaman 2 dari 7");
  });

  it("keeps a stale page inside the count", () => {
    expect(pageStatusText(9, 7)).toBe("Halaman 7 dari 7");
  });
});

describe("showsPageSizeSelect", () => {
  it.each([
    { total: 0, want: false },
    { total: 20, want: false },
    { total: 21, want: true },
  ])("is $want for $total rows", ({ total, want }) => {
    expect(showsPageSizeSelect(total)).toBe(want);
  });
});

describe("toPageSize", () => {
  it.each([
    ...PAGE_SIZES.map((size) => ({
      name: `the offered size ${size}`,
      value: String(size),
      want: size,
    })),
    { name: "a size that is not offered", value: "30", want: 20 },
    { name: "an empty value", value: "", want: 20 },
    { name: "text", value: "banyak", want: 20 },
    { name: "a negative number", value: "-50", want: 20 },
  ])("gives $want for $name", ({ value, want }) => {
    expect(toPageSize(value)).toBe(want);
  });
});
