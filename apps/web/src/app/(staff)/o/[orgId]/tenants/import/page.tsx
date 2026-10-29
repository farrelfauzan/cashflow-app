import { Suspense } from "react";

import { TenantImportScreen } from "@/components/server/tenant-import/tenant-import-screen";
import { DetailScreenSkeleton } from "@/components/server/shell/detail-screen-skeleton";

// The screen reads the session and the route's params, so it streams in behind its own boundary:
// a navigation inside the organization (where the shell is already on screen) shows the
// placeholder at once instead of waiting for the API.
export default function TenantImportPage({
  params,
}: PageProps<"/o/[orgId]/tenants/import">) {
  return (
    <Suspense fallback={<DetailScreenSkeleton />}>
      <TenantImportScreen params={params} />
    </Suspense>
  );
}
