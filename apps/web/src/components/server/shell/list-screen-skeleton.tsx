import { Skeleton } from "@/components/ui/skeleton";

import { SCREEN_LOADING_LABEL } from "./utils";

// The placeholder of a list screen (Properti, Penyewa, Anggota tim) while its data streams in:
// the title and its line, the search row, and a few rows, sized like the real screen so nothing
// jumps when it arrives.
export function ListScreenSkeleton() {
  return (
    <div
      role="status"
      aria-label={SCREEN_LOADING_LABEL}
      className="flex flex-col gap-4 md:gap-[22px]"
    >
      <div className="flex flex-col gap-1.5">
        <Skeleton className="h-8 w-40 bg-muted md:h-10 md:w-56" />
        <Skeleton className="h-5 w-64 max-w-full bg-muted md:h-6 md:w-96" />
      </div>
      <Skeleton className="h-12 w-full rounded-xl bg-muted" />
      <div className="flex flex-col gap-2.5">
        {[0, 1, 2, 3, 4].map((row) => (
          <Skeleton key={row} className="h-[76px] w-full rounded-xl bg-muted" />
        ))}
      </div>
    </div>
  );
}
