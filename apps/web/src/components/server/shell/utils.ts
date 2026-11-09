import {
  Building2,
  House,
  ReceiptText,
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
  billing: ReceiptText,
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
    key: "billing",
    label: "Penagihan",
    segment: "billing",
    permission: ["read", "invoice"],
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

// What a screen's placeholder says to screen readers while its data streams in.
export const SCREEN_LOADING_LABEL = "Memuat…";

// The phone's tab bar (board HP-Tab, approved 2026-11-08): these sections in this order, each
// only when the member may open it (the same visibleOrgMenu), then "Lainnya". The rest of the
// side menu (Anggota tim) moves into "Lainnya". iPad and desktop keep the side menu.
export const PHONE_TAB_KEYS: readonly OrgMenuItem["key"][] = [
  "home",
  "tenants",
  "billing",
  "properties",
];
export const MORE_MENU_LABEL = "Lainnya";
export const SWITCH_ORG_LABEL = "Ganti usaha";

// The sections the tab bar shows, in its order.
export function phoneTabs<T extends { key: OrgMenuItem["key"] }>(
  items: readonly T[],
): T[] {
  return PHONE_TAB_KEYS.flatMap((key) =>
    items.filter((item) => item.key === key),
  );
}

// The sections that live in "Lainnya" on the phone, in the side menu's order.
export function moreMenuSections<T extends { key: OrgMenuItem["key"] }>(
  items: readonly T[],
): T[] {
  return items.filter((item) => !PHONE_TAB_KEYS.includes(item.key));
}

// Under the member's name in "Lainnya": "Admin di Griya Sejahtera".
export function moreMenuRoleLine(roles: string, organizationName: string) {
  return roles === "" ? organizationName : `${roles} di ${organizationName}`;
}
