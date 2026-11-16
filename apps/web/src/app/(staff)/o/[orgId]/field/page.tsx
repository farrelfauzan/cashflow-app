import { Suspense } from "react";

import { FieldTodayScreen } from "@/components/server/field-today/field-today-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the route's params, so it streams in behind its own boundary: a navigation
// inside the organization (where the shell is already on screen) shows the placeholder at once
// instead of waiting for the API.
export default function FieldTodayPage({
  params,
}: PageProps<"/o/[orgId]/field">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <FieldTodayScreen params={params} />
    </Suspense>
  );
}
