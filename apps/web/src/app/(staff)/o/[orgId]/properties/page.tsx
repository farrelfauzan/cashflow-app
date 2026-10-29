import { Suspense } from "react";

import { PropertiesScreen } from "@/components/server/property/properties-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the session and the route's params, so it streams in behind its own boundary:
// a navigation inside the organization (where the shell is already on screen) shows the
// placeholder at once instead of waiting for the API.
export default function PropertiesPage({
  params,
}: PageProps<"/o/[orgId]/properties">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <PropertiesScreen params={params} />
    </Suspense>
  );
}
