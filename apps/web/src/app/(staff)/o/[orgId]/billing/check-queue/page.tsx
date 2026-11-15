import { Suspense } from "react";

import { CheckQueueScreen } from "@/components/server/check-queue/check-queue-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the route's params, so it streams in behind its own boundary: a navigation
// inside the organization (where the shell is already on screen) shows the placeholder at once
// instead of waiting for the API.
export default function CheckQueuePage({
  params,
}: PageProps<"/o/[orgId]/billing/check-queue">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <CheckQueueScreen params={params} />
    </Suspense>
  );
}
