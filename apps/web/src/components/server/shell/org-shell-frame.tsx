import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import type { ReactNode } from "react";

import { PageMotion } from "@/components/client/page-motion/page-motion";
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
import { PhoneTabBar } from "./phone-tab-bar";
import {
  loadOrgMe,
  moreMenuRoleLine,
  PICKER_PATH,
  roleSummary,
  visibleOrgMenu,
} from "./utils";

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

  const roles = roleSummary(orgMe.roles);
  const items = visibleOrgMenu(createAppAbility(orgMe.capabilities)).map(
    (item) => ({
      key: item.key,
      label: item.label,
      href: orgPath(orgId, item.segment),
    }),
  );

  return (
    <div className="flex min-h-dvh flex-col bg-background pr-[env(safe-area-inset-right)] md:flex-row">
      <SidebarRail
        defaultCollapsed={isSidebarCollapsed(
          cookieStore.get(SIDEBAR_COOKIE)?.value,
        )}
      >
        <OrgNav
          organizationName={organization.name}
          roles={roles}
          userName={me.user.name}
          currentOrgId={orgId}
          organizations={me.organizations.map(({ id, name }) => ({ id, name }))}
          items={items}
        />
      </SidebarRail>
      <MobileHeader
        organizationName={organization.name}
        detail={[me.user.name, roles].filter((part) => part !== "").join(" · ")}
      />
      {/* On the phone the page ends above the fixed tab bar (64px plus the home indicator), and
          on iOS above the floating one (66px, 24px or the home indicator under it). */}
      <main className="flex min-w-0 flex-1 flex-col px-4 pt-5 pb-[calc(env(safe-area-inset-bottom)+88px)] md:px-8 md:py-[30px] ios:max-md:pb-[calc(max(24px,env(safe-area-inset-bottom))+82px)]">
        <OrgAbilityProvider
          key={orgId}
          orgId={orgId}
          capabilities={orgMe.capabilities}
        >
          {children}
        </OrgAbilityProvider>
      </main>
      <PhoneTabBar
        items={items}
        userName={me.user.name}
        roleLine={moreMenuRoleLine(roles, organization.name)}
        canSwitch={me.organizations.length > 1}
      />
      <PageMotion />
    </div>
  );
}
