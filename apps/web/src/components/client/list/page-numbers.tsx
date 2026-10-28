import { PaginationItem } from "@/components/ui/pagination";

import { PageEllipsis } from "./page-ellipsis";
import { PageNumberLink } from "./page-number-link";
import { ELLIPSIS, pageItems } from "./utils";

type PageNumbersProps = {
  page: number;
  count: number;
  hrefFor: (page: number) => string;
};

// The numbered items of the page bar, shown on iPad (md and up) only. They are list items of the
// bar's own list, so the bar is one flat list on both sizes.
export function PageNumbers({ page, count, hrefFor }: PageNumbersProps) {
  return pageItems(page, count).map((item, index) =>
    item === ELLIPSIS ? (
      <PaginationItem
        key={`${ELLIPSIS}-${index}`}
        aria-hidden="true"
        className="hidden md:block"
      >
        <PageEllipsis />
      </PaginationItem>
    ) : (
      <PaginationItem key={item} className="hidden md:block">
        <PageNumberLink page={item} current={item === page} hrefFor={hrefFor} />
      </PaginationItem>
    ),
  );
}
