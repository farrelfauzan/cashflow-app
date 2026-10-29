import { Suspense } from "react";

import { PropertyUnitsScreen } from "@/components/server/property/property-units-screen";
import { DetailScreenSkeleton } from "@/components/server/shell/detail-screen-skeleton";

// The screen reads the session and the route's params, so it streams in behind its own boundary:
// a navigation inside the organization (where the shell is already on screen) shows the
// placeholder at once instead of waiting for the API.
export default function PropertyUnitsPage({
  params,
}: PageProps<"/o/[orgId]/properties/[propertyId]">) {
  return (
    <Suspense fallback={<DetailScreenSkeleton />}>
      <PropertyUnitsScreen params={params} />
    </Suspense>
  );
}
