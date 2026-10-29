import { Suspense } from "react";

import { MembersScreen } from "@/components/server/member/members-screen";
import { ListScreenSkeleton } from "@/components/server/shell/list-screen-skeleton";

// The screen reads the session and the route's params, so it streams in behind its own boundary:
// a navigation inside the organization (where the shell is already on screen) shows the
// placeholder at once instead of waiting for the API.
export default function MembersPage({
  params,
}: PageProps<"/o/[orgId]/members">) {
  return (
    <Suspense fallback={<ListScreenSkeleton />}>
      <MembersScreen params={params} />
    </Suspense>
  );
}
