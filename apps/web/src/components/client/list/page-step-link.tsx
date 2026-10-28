import { ChevronLeft, ChevronRight } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";

import type { StepDirection } from "./type";
import { NEXT_LABEL, PREVIOUS_LABEL } from "./utils";

type PageStepLinkProps = {
  direction: StepDirection;
  // The page to go to, or null on the first or last page, where the step is disabled.
  target: number | null;
  hrefFor: (page: number) => string;
};

// "Sebelumnya" or "Berikutnya". A link to the neighbouring page; a disabled button, not a link,
// when there is none.
export function PageStepLink({
  direction,
  target,
  hrefFor,
}: PageStepLinkProps) {
  const previous = direction === "previous";
  const label = previous ? PREVIOUS_LABEL : NEXT_LABEL;
  // The chevrons give way on the phone, so "Halaman 2 dari 7" fits between the two steps.
  const content = (
    <>
      {previous && (
        <ChevronLeft aria-hidden="true" className="hidden size-4 md:block" />
      )}
      {label}
      {!previous && (
        <ChevronRight aria-hidden="true" className="hidden size-4 md:block" />
      )}
    </>
  );
  const className = "gap-1 px-2.5 text-sm md:px-4 md:text-base";

  if (target === null) {
    return (
      <Button type="button" variant="outline" disabled className={className}>
        {content}
      </Button>
    );
  }
  return (
    <Button asChild variant="outline" className={className}>
      <Link href={hrefFor(target)}>{content}</Link>
    </Button>
  );
}
