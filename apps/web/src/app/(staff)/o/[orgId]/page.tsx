import { Suspense } from "react";

import { DashboardScreen } from "@/components/server/dashboard/dashboard-screen";
import { DetailScreenSkeleton } from "@/components/server/shell/detail-screen-skeleton";

// Beranda streams in behind its own boundary, so a navigation inside the organization shows the
// placeholder at once instead of waiting for the route's params.
export default function OrgHomePage({ params }: PageProps<"/o/[orgId]">) {
  return (
    <Suspense fallback={<DetailScreenSkeleton />}>
      <DashboardScreen params={params} />
    </Suspense>
  );
}
