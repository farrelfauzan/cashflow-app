import type { PageItem } from "./type";

// Page sizes the member can choose; the API allows 1 to 100 and answers 20 by default.
export const PAGE_SIZES = [20, 50, 100] as const;

export const DEFAULT_PAGE_SIZE = PAGE_SIZES[0];

// Up to this many pages every number is shown; beyond it the bar uses ellipses.
const ALL_PAGES_LIMIT = 7;

// Within this many pages of the first or last page, the bar keeps five numbers in a row.
const EDGE_DISTANCE = 3;
const EDGE_RUN = 5;

export const ELLIPSIS = "ellipsis";

export const PAGINATION_LABEL = "Halaman";
export const PREVIOUS_LABEL = "Sebelumnya";
export const NEXT_LABEL = "Berikutnya";
export const PAGE_SIZE_LABEL = "Baris per halaman";

const NUMBER_FORMAT = new Intl.NumberFormat("id-ID", {
  useGrouping: true,
  maximumFractionDigits: 0,
});

// Number of pages for a list of `total` rows; an empty list still has one page.
export function pageCount(total: number, pageSize: number): number {
  if (!(pageSize > 0) || !(total > 0)) {
    return 1;
  }
  return Math.max(1, Math.ceil(total / pageSize));
}

// Keeps a page inside 1..count, so a stale link (page 9 of 3) still shows a real page.
export function clampPage(page: number, count: number): number {
  if (!Number.isFinite(page)) {
    return 1;
  }
  return Math.min(Math.max(1, Math.trunc(page)), Math.max(1, count));
}

function range(from: number, to: number): number[] {
  return Array.from({ length: to - from + 1 }, (_, i) => from + i);
}

// The numbers of the page bar, with "ellipsis" where pages are skipped: the first and last page,
// the current one and its neighbours. Near an edge it keeps five numbers in a row. A gap of one
// page shows that page, so an ellipsis never stands for a single page and two never sit side by
// side.
export function pageItems(page: number, count: number): PageItem[] {
  const last = Math.max(1, Math.trunc(count));
  if (last <= ALL_PAGES_LIMIT) {
    return range(1, last);
  }
  const current = clampPage(page, last);

  const shown = new Set<number>([1, last]);
  let around: number[];
  if (current <= EDGE_DISTANCE) {
    around = range(1, EDGE_RUN);
  } else if (current > last - EDGE_DISTANCE) {
    around = range(last - EDGE_RUN + 1, last);
  } else {
    around = range(current - 1, current + 1);
  }
  around.forEach((n) => shown.add(n));

  const sorted = [...shown].sort((a, b) => a - b);
  const items: PageItem[] = [];
  sorted.forEach((n, i) => {
    const previous = sorted[i - 1];
    if (previous !== undefined) {
      const gap = n - previous - 1;
      if (gap === 1) {
        items.push(previous + 1);
      } else if (gap > 1) {
        items.push(ELLIPSIS);
      }
    }
    items.push(n);
  });
  return items;
}

// "Menampilkan 21–40 dari 134"; null for an empty list. A page past the end shows the last page.
export function rangeText(
  page: number,
  pageSize: number,
  total: number,
): string | null {
  if (!(total > 0) || !(pageSize > 0)) {
    return null;
  }
  const current = clampPage(page, pageCount(total, pageSize));
  const first = (current - 1) * pageSize + 1;
  const last = Math.min(current * pageSize, total);
  return `Menampilkan ${NUMBER_FORMAT.format(first)}–${NUMBER_FORMAT.format(last)} dari ${NUMBER_FORMAT.format(total)}`;
}

// The compact status on the phone, where page numbers do not fit.
export function pageStatusText(page: number, count: number): string {
  return `Halaman ${clampPage(page, count)} dari ${Math.max(1, count)}`;
}

// The page-size select only helps when the list is longer than the smallest size.
export function showsPageSizeSelect(total: number): boolean {
  return total > PAGE_SIZES[0];
}

// What the select sends back as text; any value that is not an offered size gives 20.
export function toPageSize(value: string): number {
  const size = Number(value);
  return PAGE_SIZES.find((allowed) => allowed === size) ?? DEFAULT_PAGE_SIZE;
}

// The URL search params a paginated list keeps its state in, so back, reload and a shared link
// show the same rows. Defaults are left out of the URL (no page=1, no pageSize=20).
export const PAGE_PARAM = "page";
export const PAGE_SIZE_PARAM = "pageSize";
export const SEARCH_PARAM = "q";
export const SORT_PARAM = "sort";

// How long the search waits after the last key before it asks the API.
export const SEARCH_DELAY_MS = 300;

// The API allows pages up to this number; a larger one in the URL is not a page.
const MAX_PAGE = 1_000_000;

// The page in the URL: a whole number from 1, anything else (missing, "abc", "0", "2.5") is 1.
// A page past the last is kept: the list answers it with no rows and the right total, and the
// screen then goes to the last page.
export function readPage(params: URLSearchParams): number {
  const raw = params.get(PAGE_PARAM);
  if (raw === null || !/^[1-9][0-9]*$/.test(raw)) {
    return 1;
  }
  const page = Number(raw);
  return page <= MAX_PAGE ? page : 1;
}

// The page size in the URL when it is one of the offered sizes, otherwise 20.
export function readPageSize(params: URLSearchParams): number {
  const raw = params.get(PAGE_SIZE_PARAM);
  return raw === null ? DEFAULT_PAGE_SIZE : toPageSize(raw);
}

// The search as typed (the field shows it back); empty when there is none.
export function readSearch(params: URLSearchParams): string {
  return params.get(SEARCH_PARAM) ?? "";
}

// What the API gets for a search: the trimmed text, or nothing for a blank one.
export function searchQuery(search: string): string | undefined {
  const q = search.trim();
  return q === "" ? undefined : q;
}

// A param whose value must be one of `allowed` (a status, a sort); anything else counts as not
// set, so a hand-edited URL never makes the API answer 400.
export function readChoice<T extends string>(
  params: URLSearchParams,
  name: string,
  allowed: readonly T[],
): T | undefined {
  const raw = params.get(name);
  return allowed.find((value) => value === raw);
}

// The address after `changes` (null removes a param). Changing anything but the page (a search,
// a filter, the sort, the page size) goes back to page 1, because the old page number means
// nothing in the new list. Defaults are dropped, and the other params keep their order.
export function listHref(
  pathname: string,
  params: URLSearchParams,
  changes: Readonly<Record<string, string | number | null>>,
): string {
  const next = new URLSearchParams(params);
  const resetsPage = Object.keys(changes).some((name) => name !== PAGE_PARAM);
  if (resetsPage) {
    next.delete(PAGE_PARAM);
  }
  for (const [name, value] of Object.entries(changes)) {
    if (value === null || value === "") {
      next.delete(name);
    } else {
      next.set(name, String(value));
    }
  }
  if (next.get(PAGE_PARAM) === "1") {
    next.delete(PAGE_PARAM);
  }
  if (next.get(PAGE_SIZE_PARAM) === String(DEFAULT_PAGE_SIZE)) {
    next.delete(PAGE_SIZE_PARAM);
  }
  const query = next.toString();
  return query === "" ? pathname : `${pathname}?${query}`;
}

// The page to go to when the API answered a page past the end: the last page that has rows.
// null when the current page is fine (it has rows, or the whole list is empty).
export function pageAfterOverflow(
  page: number,
  pageSize: number,
  total: number,
  rowsOnPage: number,
): number | null {
  if (rowsOnPage > 0 || total === 0) {
    return null;
  }
  const last = pageCount(total, pageSize);
  return page > last ? last : null;
}
