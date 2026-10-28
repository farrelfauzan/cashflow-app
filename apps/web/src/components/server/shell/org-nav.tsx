import { ArrowLeftRight, Building2, House, Users, UserCog } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { LogoutButton } from "@/components/client/auth/logout-button";
import { OrgNavLink } from "@/components/client/shell/org-nav-link";
import { OrgTile } from "@/components/server/organization/org-tile";
import type { OrgMenuItem } from "@/lib/organization/org-menu";

export type OrgNavItem = {
  key: OrgMenuItem["key"];
  label: string;
  href: string;
};

type OrgNavProps = {
  organizationName: string;
  roles: string;
  userName: string;
  canSwitch: boolean;
  switchHref: string;
  items: readonly OrgNavItem[];
};

const ICONS: Record<OrgNavItem["key"], ReactNode> = {
  home: <House aria-hidden="true" className="size-5" />,
  properties: <Building2 aria-hidden="true" className="size-5" />,
  tenants: <Users aria-hidden="true" className="size-5" />,
  members: <UserCog aria-hidden="true" className="size-5" />,
};

// The side menu (board Members.dc.html): the organization and the member's roles, "Ganti
// usaha" for a member of several organizations, the sections they may open, and who is signed
// in with "Keluar". The same content fills the phone's menu sheet, where the header leaves room
// for the sheet's close button.
export function OrgNav({
  organizationName,
  roles,
  userName,
  canSwitch,
  switchHref,
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
      {canSwitch && (
        <Link
          href={switchHref}
          className="mb-2.5 flex min-h-11 items-center gap-2 rounded-[10px] px-2.5 text-[15px] font-semibold text-primary hover:bg-accent/60"
        >
          <ArrowLeftRight aria-hidden="true" className="size-[18px]" />
          Ganti usaha
        </Link>
      )}
      <ul className="flex flex-col gap-0.5">
        {items.map((item) => (
          <li key={item.key}>
            <OrgNavLink href={item.href} exact={item.key === "home"}>
              {ICONS[item.key]}
              {item.label}
            </OrgNavLink>
          </li>
        ))}
      </ul>
      <div className="flex-1" />
      <div className="flex flex-col gap-0.5 border-t px-1 pt-3">
        <span className="truncate px-2 text-[15px] font-semibold">
          {userName}
        </span>
        <LogoutButton variant="menu" />
      </div>
    </nav>
  );
}
