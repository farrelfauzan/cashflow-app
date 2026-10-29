// One entry of the page numbers bar: a page number, or a gap standing for two or more pages.
export type PageItem = number | "ellipsis";

export type StepDirection = "previous" | "next";

// A paginated list's state in the URL (useListUrl).
export type ListUrl = {
  searchParams: URLSearchParams;
  // The link to a page of the list, keeping the search, filters, sort and page size.
  hrefFor: (page: number) => string;
  // Sets (or with null, removes) params; anything but the page goes back to page 1.
  change: (
    changes: Readonly<Record<string, string | number | null>>,
    options?: { replace?: boolean },
  ) => void;
};

// One choice of a list's "Urutkan" select: the value the API takes as `sort` and its label.
export type SortOption<T extends string = string> = {
  value: T;
  label: string;
};

// A list's sort choices; the first is the API's default, which stays out of the URL.
export type SortOptions<T extends string = string> = readonly [
  SortOption<T>,
  ...SortOption<T>[],
];

// The sorts of the lists that order by name or by when the row was added (members and tenants).
export type AddedSort = "name" | "-name" | "createdAt" | "-createdAt";
