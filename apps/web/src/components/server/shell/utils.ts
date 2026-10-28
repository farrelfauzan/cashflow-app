import {
  Building2,
  House,
  Users,
  UserCog,
  type LucideIcon,
} from "lucide-react";

import { ApiError } from "@/lib/api/errors";
import { getOrgMe } from "@/lib/api/generated/members/members";
import type { RoleRef } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import type { OrgMenuItem, OrgNavItem } from "./type";

export const PICKER_PATH = "/organizations";

export const ORG_NAV_ICONS: Record<OrgNavItem["key"], LucideIcon> = {
  home: House,
  properties: Building2,
  tenants: Users,
  members: UserCog,
};

// getOrgMe answers 404 for an organization the user is not an active member of (and 400 for
// an id that is not one); both go back to the picker, which lists only active memberships, so
// this cannot loop. The call also records the member's first sign-in in this organization.
export async function loadOrgMe(orgId: string) {
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

// The side menu inside an organization. An item shows when the member's ability (from
// getOrgMe's capabilities) grants its permission, on every record or on some ("conditional").
// Never by role name: a custom role or a property limit changes what shows without touching this
// list. Hiding is visibility only; the API checks every request.
export const ORG_MENU: readonly OrgMenuItem[] = [
  { key: "home", label: "Beranda", segment: "", permission: null },
  {
    key: "properties",
    label: "Properti",
    segment: "properties",
    permission: ["read", "property"],
  },
  {
    key: "tenants",
    label: "Penyewa",
    segment: "tenants",
    permission: ["read", "tenant"],
  },
  {
    key: "members",
    label: "Anggota tim",
    segment: "members",
    permission: ["read", "member"],
  },
];

export function visibleOrgMenu(ability: AppAbility): OrgMenuItem[] {
  return ORG_MENU.filter(
    (item) => item.permission === null || ability.can(...item.permission),
  );
}

// The roles under the organization's name in the menu ("Admin", "Karyawan, Admin"). Display
// only: what the member may do comes from capabilities, never from these names. A role held
// for several properties shows once.
export function roleSummary(roles: readonly RoleRef[]): string {
  return [...new Set(roles.map((role) => role.name))].join(", ");
}
