"use client";

import { Pagination } from "@/components/ui/pagination";

import { PageControls } from "./page-controls";
import { PageSizeSelect } from "./page-size-select";
import {
  clampPage,
  PAGINATION_LABEL,
  pageCount,
  rangeText,
  showsPageSizeSelect,
} from "./utils";

type ListPaginationProps = {
  // 1-based, as the API answers it.
  page: number;
  pageSize: number;
  total: number;
  // The link to a page, so back, reload and sharing work. The caller builds it from its URL.
  hrefFor: (page: number) => string;
  onPageSizeChange: (pageSize: number) => void;
};

// The bar under a paginated table: which rows are shown, the page controls when there is more
// than one page, and the page size when the list is longer than the smallest size. Shows nothing
// for an empty list. On the phone the controls are compact and sit above the range and the size.
export function ListPagination({
  page,
  pageSize,
  total,
  hrefFor,
  onPageSizeChange,
}: ListPaginationProps) {
  const summary = rangeText(page, pageSize, total);
  if (summary === null) {
    return null;
  }
  const count = pageCount(total, pageSize);
  const current = clampPage(page, count);

  return (
    <Pagination
      aria-label={PAGINATION_LABEL}
      className="mx-0 flex-col gap-3 md:flex-row md:items-center md:justify-between md:gap-4"
    >
      {count > 1 && (
        <div className="md:order-2">
          <PageControls page={current} count={count} hrefFor={hrefFor} />
        </div>
      )}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 md:order-1">
        <p className="text-sm text-muted-foreground md:text-base">{summary}</p>
        {showsPageSizeSelect(total) && (
          <PageSizeSelect
            pageSize={pageSize}
            onPageSizeChange={onPageSizeChange}
          />
        )}
      </div>
    </Pagination>
  );
}
