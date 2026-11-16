"use client";

import { Skeleton } from "@/components/ui/skeleton";

import { FIELD_PROMISE_LOADING_LABEL } from "./utils";

type FieldPromiseSkeletonProps = {
  // The body's classes for the drawer (PROMISE_SHEET_CLASSES), so nothing jumps when it loads.
  className: string;
};

// "Catat janji bayar" while what a promise may name loads: the intro line, the two choices, the
// date and the amount, as placeholders.
export function FieldPromiseSkeleton({ className }: FieldPromiseSkeletonProps) {
  return (
    <div
      role="status"
      aria-label={FIELD_PROMISE_LOADING_LABEL}
      className={className}
    >
      <Skeleton className="h-5 w-3/4 rounded-md bg-muted" />
      <Skeleton className="h-16 w-full rounded-xl bg-muted" />
      <Skeleton className="h-16 w-full rounded-xl bg-muted" />
      <Skeleton className="h-[84px] w-full rounded-xl bg-muted" />
      <Skeleton className="h-[84px] w-full rounded-xl bg-muted" />
    </div>
  );
}
