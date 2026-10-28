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
