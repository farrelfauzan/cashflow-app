import { Suspense } from "react";

import { FieldVisitsScreen } from "@/components/server/field-visits/field-visits-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the route's params, so it streams in behind its own boundary: a navigation
// inside the organization (where the shell is already on screen) shows the placeholder at once
// instead of waiting for the API.
export default function FieldVisitsPage({
  params,
}: PageProps<"/o/[orgId]/field/visits">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <FieldVisitsScreen params={params} />
    </Suspense>
  );
}
