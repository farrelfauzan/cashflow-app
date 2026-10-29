import { PaginationContent, PaginationItem } from "@/components/ui/pagination";

import { PageNumbers } from "./page-numbers";
import { PageStepLink } from "./page-step-link";
import { pageStatusText } from "./utils";

type PageControlsProps = {
  // Already inside 1..count.
  page: number;
  count: number;
  hrefFor: (page: number) => string;
};

// "Sebelumnya", the page numbers and "Berikutnya". On the phone the numbers give way to
// "Halaman 2 dari 7".
export function PageControls({ page, count, hrefFor }: PageControlsProps) {
  return (
    <PaginationContent className="w-full justify-between gap-1 md:w-auto md:justify-end">
      <PaginationItem>
        <PageStepLink
          direction="previous"
          target={page > 1 ? page - 1 : null}
          hrefFor={hrefFor}
        />
      </PaginationItem>
      <PaginationItem className="px-1 text-center text-sm font-medium md:hidden">
        {pageStatusText(page, count)}
      </PaginationItem>
      <PageNumbers page={page} count={count} hrefFor={hrefFor} />
      <PaginationItem>
        <PageStepLink
          direction="next"
          target={page < count ? page + 1 : null}
          hrefFor={hrefFor}
        />
      </PaginationItem>
    </PaginationContent>
  );
}
