import { Suspense } from "react";

import { DepositCountScreen } from "@/components/server/deposit-count/deposit-count-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the route's params, so it streams in behind its own boundary: a navigation
// inside the organization (where the shell is already on screen) shows the placeholder at once
// instead of waiting for the API.
export default function DepositCountPage({
  params,
}: PageProps<"/o/[orgId]/billing/deposits">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <DepositCountScreen params={params} />
    </Suspense>
  );
}
