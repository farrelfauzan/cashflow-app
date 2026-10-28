import Link from "next/link";

import { Button } from "@/components/ui/button";

type PageNumberLinkProps = {
  page: number;
  current: boolean;
  hrefFor: (page: number) => string;
};

// One number of the page bar. The current page is plain text marked aria-current, not a link.
export function PageNumberLink({
  page,
  current,
  hrefFor,
}: PageNumberLinkProps) {
  if (current) {
    return (
      <span
        aria-current="page"
        className="inline-flex h-11 min-w-11 items-center justify-center rounded-md bg-primary px-3 text-base font-semibold text-primary-foreground"
      >
        {page}
      </span>
    );
  }
  return (
    <Button
      asChild
      variant="ghost"
      className="min-w-11 px-3 font-medium text-foreground"
    >
      <Link href={hrefFor(page)}>{page}</Link>
    </Button>
  );
}
