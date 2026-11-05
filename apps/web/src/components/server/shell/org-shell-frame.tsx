import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { OrgAbilityProvider } from "@/components/client/shell/org-ability-provider";
import { SidebarRail } from "@/components/client/shell/sidebar-rail";
import {
  isSidebarCollapsed,
  SIDEBAR_COOKIE,
} from "@/components/client/shell/utils";
import { getMe } from "@/lib/api/generated/auth/auth";
import { createAppAbility } from "@/lib/capabilities";
import { orgPath } from "@/lib/org-path";

import { MobileHeader } from "./mobile-header";
import { OrgNav } from "./org-nav";
import { loadOrgMe, PICKER_PATH, roleSummary, visibleOrgMenu } from "./utils";

type OrgShellFrameProps = {
  params: Promise<{ orgId: string }>;
  children: ReactNode;
};

export async function OrgShellFrame({ params, children }: OrgShellFrameProps) {
  const { orgId } = await params;
  const [me, orgMe, cookieStore] = await Promise.all([
    getMe(),
    loadOrgMe(orgId),
    cookies(),
  ]);
  const organization = me.organizations.find((o) => o.id === orgId);
  if (orgMe === null || organization === undefined) redirect(PICKER_PATH);

  const nav = (
    <OrgNav
      organizationName={organization.name}
      roles={roleSummary(orgMe.roles)}
      userName={me.user.name}
      currentOrgId={orgId}
      organizations={me.organizations.map(({ id, name }) => ({ id, name }))}
      items={visibleOrgMenu(createAppAbility(orgMe.capabilities)).map(
        (item) => ({
          key: item.key,
          label: item.label,
          href: orgPath(orgId, item.segment),
        }),
      )}
    />
  );

  return (
    <div className="flex min-h-dvh flex-col bg-background md:flex-row">
      <SidebarRail
        defaultCollapsed={isSidebarCollapsed(
          cookieStore.get(SIDEBAR_COOKIE)?.value,
        )}
      >
        {nav}
      </SidebarRail>
      <MobileHeader
        organizationName={organization.name}
        detail={[me.user.name, roleSummary(orgMe.roles)]
          .filter((part) => part !== "")
          .join(" · ")}
      >
        {nav}
      </MobileHeader>
      <main className="flex min-w-0 flex-1 flex-col px-4 pt-5 pb-6 md:px-8 md:py-[30px]">
        <OrgAbilityProvider
          key={orgId}
          orgId={orgId}
          capabilities={orgMe.capabilities}
        >
          {children}
        </OrgAbilityProvider>
      </main>
    </div>
  );
}
