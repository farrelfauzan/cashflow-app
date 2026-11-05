"use client";

import { Skeleton } from "@/components/ui/skeleton";

import { PaymentPageFrame } from "./payment-page-frame";
import { LOADING_TEXT } from "./utils";

// The quiet placeholder while the page loads (also the route's Suspense fallback): the header
// and the summary card, sized like the real page so nothing jumps when it arrives.
export function PaymentPageLoading() {
  return (
    <PaymentPageFrame>
      <div
        role="status"
        aria-label={LOADING_TEXT}
        className="flex flex-col gap-4 md:gap-5"
      >
        <div className="flex flex-col gap-2">
          <Skeleton className="h-4 w-32 bg-muted" />
          <Skeleton className="h-8 w-64 max-w-full bg-muted" />
          <Skeleton className="h-5 w-52 max-w-full bg-muted" />
        </div>
        <Skeleton className="h-40 w-full rounded-xl bg-muted" />
        <Skeleton className="h-48 w-full rounded-xl bg-muted" />
      </div>
    </PaymentPageFrame>
  );
}
