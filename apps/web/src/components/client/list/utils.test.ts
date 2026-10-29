import { describe, expect, it } from "vitest";

import type { PageItem, SortOptions } from "./type";
import {
  ADDED_SORT_OPTIONS,
  clampPage,
  listHref,
  pageAfterOverflow,
  readChoice,
  readPage,
  readPageSize,
  readSearch,
  readSort,
  searchQuery,
  sortParam,
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

function params(query: string): URLSearchParams {
  return new URLSearchParams(query);
}

describe("readPage", () => {
  it.each([
    { name: "no page", query: "", want: 1 },
    { name: "page 3", query: "page=3", want: 3 },
    { name: "page 0", query: "page=0", want: 1 },
    { name: "a negative page", query: "page=-2", want: 1 },
    { name: "a fraction", query: "page=2.5", want: 1 },
    { name: "a leading zero", query: "page=02", want: 1 },
    { name: "text", query: "page=abc", want: 1 },
    { name: "past the API's limit", query: "page=1000001", want: 1 },
    { name: "a page past the last, kept", query: "page=99", want: 99 },
  ])("is $want for $name", ({ query, want }) => {
    expect(readPage(params(query))).toBe(want);
  });
});

describe("readPageSize", () => {
  it.each([
    { name: "no size", query: "", want: 20 },
    { name: "an offered size", query: "pageSize=50", want: 50 },
    { name: "the largest size", query: "pageSize=100", want: 100 },
    { name: "a size not offered", query: "pageSize=30", want: 20 },
    { name: "over the API's limit", query: "pageSize=500", want: 20 },
  ])("is $want for $name", ({ query, want }) => {
    expect(readPageSize(params(query))).toBe(want);
  });
});

describe("readSearch and searchQuery", () => {
  it("reads the search as typed and sends it trimmed", () => {
    const search = readSearch(params("q=%20Gita%20"));
    expect(search).toBe(" Gita ");
    expect(searchQuery(search)).toBe("Gita");
  });

  it("cuts a search to the API's 100 characters, counting characters", () => {
    expect(searchQuery("a".repeat(150))).toBe("a".repeat(100));
    // Each emoji is two UTF-16 units but one character for the API.
    expect(searchQuery("🏠".repeat(120))).toBe("🏠".repeat(100));
    expect(searchQuery(`${"a".repeat(99)} b`)).toBe("a".repeat(99));
  });

  it("sends nothing for a missing or blank search", () => {
    expect(readSearch(params(""))).toBe("");
    expect(searchQuery("")).toBeUndefined();
    expect(searchQuery("   ")).toBeUndefined();
  });
});

describe("readChoice", () => {
  const statuses = ["active", "ended"] as const;

  it("reads an allowed value", () => {
    expect(readChoice(params("status=ended"), "status", statuses)).toBe(
      "ended",
    );
  });

  it.each([
    { name: "a missing param", query: "" },
    { name: "a value outside the list", query: "status=deleted" },
    { name: "a value in another case", query: "status=Active" },
  ])("is undefined for $name", ({ query }) => {
    expect(readChoice(params(query), "status", statuses)).toBeUndefined();
  });
});

describe("sort", () => {
  const options: SortOptions = [
    { value: "label", label: "Nama unit A–Z" },
    { value: "-label", label: "Nama unit Z–A" },
  ];

  it.each([
    { name: "a missing param", query: "", want: "label" },
    { name: "the default", query: "sort=label", want: "label" },
    { name: "an offered value", query: "sort=-label", want: "-label" },
    { name: "a value outside the list", query: "sort=name", want: "label" },
    { name: "a value in another case", query: "sort=-Label", want: "label" },
    { name: "an empty value", query: "sort=", want: "label" },
  ])("reads $name", ({ query, want }) => {
    expect(readSort(params(query), options)).toBe(want);
  });

  it("leaves the default out of the URL and keeps the others", () => {
    expect(sortParam("label", options)).toBeNull();
    expect(sortParam("-label", options)).toBe("-label");
  });

  it("goes back to page 1 and keeps the search and filters on a change", () => {
    const path = "/orgs/o1/tenants";
    const current = params("page=3&q=gita&status=active");
    expect(
      listHref(path, current, { sort: sortParam("-label", options) }),
    ).toBe(`${path}?q=gita&status=active&sort=-label`);
    expect(
      listHref(path, params("page=3&q=gita&sort=-label"), {
        sort: sortParam("label", options),
      }),
    ).toBe(`${path}?q=gita`);
  });

  it("offers the members and tenants sorts, the API default first", () => {
    expect(ADDED_SORT_OPTIONS.map((o) => [o.value, o.label])).toEqual([
      ["name", "Nama A–Z"],
      ["-name", "Nama Z–A"],
      ["-createdAt", "Terbaru ditambahkan"],
      ["createdAt", "Terlama ditambahkan"],
    ]);
    expect(readSort(params("sort=-createdAt"), ADDED_SORT_OPTIONS)).toBe(
      "-createdAt",
    );
    expect(readSort(params("sort=email"), ADDED_SORT_OPTIONS)).toBe("name");
  });
});

describe("listHref", () => {
  const path = "/orgs/o1/tenants";

  it.each<{
    name: string;
    query: string;
    changes: Record<string, string | number | null>;
    want: string;
  }>([
    {
      name: "goes to a page and keeps the search and filters",
      query: "q=gita&status=active",
      changes: { page: 3 },
      want: `${path}?q=gita&status=active&page=3`,
    },
    {
      name: "drops page 1",
      query: "q=gita&page=3",
      changes: { page: 1 },
      want: `${path}?q=gita`,
    },
    {
      name: "a new search goes back to page 1",
      query: "page=4&status=active",
      changes: { q: "nanda" },
      want: `${path}?status=active&q=nanda`,
    },
    {
      name: "a filter change goes back to page 1",
      query: "page=4&q=gita",
      changes: { status: "ended" },
      want: `${path}?q=gita&status=ended`,
    },
    {
      name: "a page size change goes back to page 1",
      query: "page=4",
      changes: { pageSize: 50 },
      want: `${path}?pageSize=50`,
    },
    {
      name: "drops the default page size",
      query: "pageSize=50&page=2",
      changes: { pageSize: 20 },
      want: path,
    },
    {
      name: "null removes a param",
      query: "q=gita&collectorDisabled=true&page=2",
      changes: { collectorDisabled: null },
      want: `${path}?q=gita`,
    },
    {
      name: "an empty string removes a param",
      query: "q=gita",
      changes: { q: "" },
      want: path,
    },
    {
      name: "clears everything",
      query: "q=gita&status=active&page=2",
      changes: { q: null, status: null },
      want: path,
    },
  ])("$name", ({ query, changes, want }) => {
    expect(listHref(path, params(query), changes)).toBe(want);
  });

  it("does not change the params it is given", () => {
    const current = params("q=gita&page=2");
    listHref(path, current, { q: "nanda" });
    expect(current.toString()).toBe("q=gita&page=2");
  });
});

describe("pageAfterOverflow", () => {
  it.each([
    {
      name: "a page with rows",
      page: 2,
      pageSize: 20,
      total: 45,
      rows: 20,
      want: null,
    },
    {
      name: "an empty list",
      page: 1,
      pageSize: 20,
      total: 0,
      rows: 0,
      want: null,
    },
    {
      name: "a page past the last",
      page: 9,
      pageSize: 20,
      total: 45,
      rows: 0,
      want: 3,
    },
    {
      name: "the last page emptied by a change",
      page: 3,
      pageSize: 20,
      total: 40,
      rows: 0,
      want: 2,
    },
  ])("is $want for $name", ({ page, pageSize, total, rows, want }) => {
    expect(pageAfterOverflow(page, pageSize, total, rows)).toBe(want);
  });
});
