import { Suspense, type ReactNode } from "react";

import { OrgShellFrame } from "./org-shell-frame";
import { OrgShellSkeleton } from "./org-shell-skeleton";

type OrgShellProps = {
  params: Promise<{ orgId: string }>;
  children: ReactNode;
};

// The frame of every page inside an organization. It needs the member's capabilities, so it
// streams in behind one boundary.
export function OrgShell({ params, children }: OrgShellProps) {
  return (
    <Suspense fallback={<OrgShellSkeleton />}>
      <OrgShellFrame params={params}>{children}</OrgShellFrame>
    </Suspense>
  );
}
