import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/errors";
import { getMe } from "@/lib/api/generated/auth/auth";
import { getOrgMe } from "@/lib/api/generated/members/members";
import { orgPath, visibleOrgMenu } from "@/lib/organization/org-menu";
import { roleSummary } from "@/lib/organization/role-summary";

import { MobileHeader } from "./mobile-header";
import { OrgNav } from "./org-nav";

const PICKER_PATH = "/organizations";

type OrgShellFrameProps = {
  params: Promise<{ orgId: string }>;
  children: ReactNode;
};

// getOrgMe answers 404 for an organization the user is not an active member of (and 400 for
// an id that is not one); both go back to the picker, which lists only active memberships, so
// this cannot loop. The call also records the member's first sign-in in this organization.
async function loadOrgMe(orgId: string) {
  try {
    return await getOrgMe(orgId);
  } catch (error) {
    if (
      error instanceof ApiError &&
      (error.status === 404 || error.status === 400)
    ) {
      return null;
    }
    throw error;
  }
}

export async function OrgShellFrame({ params, children }: OrgShellFrameProps) {
  const { orgId } = await params;
  const [me, orgMe] = await Promise.all([getMe(), loadOrgMe(orgId)]);
  const organization = me.organizations.find((o) => o.id === orgId);
  if (orgMe === null || organization === undefined) redirect(PICKER_PATH);

  const nav = (
    <OrgNav
      organizationName={organization.name}
      roles={roleSummary(orgMe.roles)}
      userName={me.user.name}
      canSwitch={me.organizations.length > 1}
      switchHref={PICKER_PATH}
      items={visibleOrgMenu(orgMe.capabilities).map((item) => ({
        key: item.key,
        label: item.label,
        href: orgPath(orgId, item.segment),
      }))}
    />
  );

  return (
    <div className="flex min-h-dvh flex-col bg-background md:flex-row">
      <aside className="sticky top-0 hidden h-dvh w-[228px] shrink-0 border-r bg-secondary/60 md:block">
        {nav}
      </aside>
      <MobileHeader
        organizationName={organization.name}
        detail={[me.user.name, roleSummary(orgMe.roles)]
          .filter((part) => part !== "")
          .join(" · ")}
      >
        {nav}
      </MobileHeader>
      <main className="flex min-w-0 flex-1 flex-col px-4 pt-5 pb-6 md:px-8 md:py-[30px]">
        {children}
      </main>
    </div>
  );
}
