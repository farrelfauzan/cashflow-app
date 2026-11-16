"use client";

import { Skeleton } from "@/components/ui/skeleton";

// The list's placeholder while it loads, sized like its rows so nothing jumps.
export function FieldTodaySkeleton() {
  return (
    <div role="status" aria-label="Memuat…" className="flex flex-col gap-2.5">
      {[0, 1, 2].map((row) => (
        <Skeleton
          key={row}
          className="h-[108px] w-full rounded-[14px] bg-muted"
        />
      ))}
    </div>
  );
}
