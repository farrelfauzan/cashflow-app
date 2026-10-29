import { Skeleton } from "@/components/ui/skeleton";

import { SCREEN_LOADING_LABEL } from "./utils";

// The placeholder of a screen about one thing (a property, a tenant, the import) while its data
// streams in: the back link, the title and its line, and one large card.
export function DetailScreenSkeleton() {
  return (
    <div
      role="status"
      aria-label={SCREEN_LOADING_LABEL}
      className="flex flex-col gap-4 md:gap-[22px]"
    >
      <div className="flex flex-col gap-1.5">
        <Skeleton className="h-6 w-24 bg-muted" />
        <Skeleton className="h-8 w-56 max-w-full bg-muted md:h-10 md:w-80" />
        <Skeleton className="h-5 w-48 bg-muted md:h-6 md:w-72" />
      </div>
      <Skeleton className="h-64 w-full rounded-2xl bg-muted" />
    </div>
  );
}
