import { LogoutButton } from "@/components/client/auth/logout-button";
import { OrgNavLink } from "@/components/client/shell/org-nav-link";
import { OrgSwitchSelect } from "@/components/client/shell/org-switch-select";
import type { OrgSwitchOption } from "@/components/client/shell/type";
import { OrgTile } from "@/components/server/organization/org-tile";

import type { OrgNavItem } from "./type";
import { ORG_NAV_ICONS } from "./utils";

type OrgNavProps = {
  organizationName: string;
  roles: string;
  userName: string;
  currentOrgId: string;
  // The member's organizations; "Ganti usaha" shows only when there are several.
  organizations: readonly OrgSwitchOption[];
  items: readonly OrgNavItem[];
};

// The side menu (board Members.dc.html): the organization and the member's roles, the sections
// they may open, then "Ganti usaha" for a member of several organizations (a select above the
// footer, moved there on the user's request) and who is signed in with "Keluar". The same
// content fills the phone's menu sheet, where the header leaves room for the sheet's close
// button.
export function OrgNav({
  organizationName,
  roles,
  userName,
  currentOrgId,
  organizations,
  items,
}: OrgNavProps) {
  return (
    <nav
      aria-label="Menu utama"
      className="flex h-full flex-col gap-0.5 px-3 py-[18px]"
    >
      <div className="flex items-center gap-2.5 pr-12 pb-1.5 pl-1 md:pr-1">
        <OrgTile
          name={organizationName}
          tone="solid"
          className="size-[38px] rounded-[10px] text-[15px]"
        />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="truncate text-[15px] font-bold">
            {organizationName}
          </span>
          {roles !== "" && (
            <span className="truncate text-sm text-muted-foreground">
              {roles}
            </span>
          )}
        </div>
      </div>
      <ul className="flex flex-col gap-0.5">
        {items.map((item) => {
          const Icon = ORG_NAV_ICONS[item.key];
          return (
            <li key={item.key}>
              <OrgNavLink href={item.href} exact={item.key === "home"}>
                <Icon aria-hidden="true" className="size-5" />
                {item.label}
              </OrgNavLink>
            </li>
          );
        })}
      </ul>
      <div className="flex-1" />
      {organizations.length > 1 && (
        <OrgSwitchSelect
          currentOrgId={currentOrgId}
          organizations={organizations}
        />
      )}
      <div className="flex flex-col gap-0.5 border-t px-1 pt-3">
        <span className="truncate px-2 text-[15px] font-semibold">
          {userName}
        </span>
        <LogoutButton variant="menu" />
      </div>
    </nav>
  );
}
