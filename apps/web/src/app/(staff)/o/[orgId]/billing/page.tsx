import { Suspense } from "react";

import { BillingScreen } from "@/components/server/invoice/billing-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the route's params, so it streams in behind its own boundary: a navigation
// inside the organization (where the shell is already on screen) shows the placeholder at once
// instead of waiting for the API.
export default function BillingPage({
  params,
}: PageProps<"/o/[orgId]/billing">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <BillingScreen params={params} />
    </Suspense>
  );
}
