import { Suspense } from "react";

import { CashDepositScreen } from "@/components/server/deposit/cash-deposit-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the route's params, so it streams in behind its own boundary: a navigation
// inside the organization (where the shell is already on screen) shows the placeholder at once
// instead of waiting for the API.
export default function CashDepositPage({
  params,
}: PageProps<"/o/[orgId]/field/deposit">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <CashDepositScreen params={params} />
    </Suspense>
  );
}
