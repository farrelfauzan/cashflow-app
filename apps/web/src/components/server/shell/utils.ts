import {
  Banknote,
  Building2,
  House,
  MapPin,
  Receipt,
  ReceiptText,
  Users,
  UserCog,
  type LucideIcon,
} from "lucide-react";
import { cache } from "react";

import { ApiError } from "@/lib/api/errors";
import { getOrgMe } from "@/lib/api/generated/members/members";
import type { RoleRef } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import type { OrgMenuItem, OrgNavItem } from "./type";

export const PICKER_PATH = "/organizations";

export const ORG_NAV_ICONS: Record<OrgNavItem["key"], LucideIcon> = {
  field: Receipt,
  visits: MapPin,
  deposit: Banknote,
  home: House,
  properties: Building2,
  tenants: Users,
  billing: ReceiptText,
  members: UserCog,
};

// getOrgMe answers 404 for an organization the user is not an active member of (and 400 for
// an id that is not one); both go back to the picker, which lists only active memberships, so
// this cannot loop. The call also records the member's first sign-in in this organization.
// Cached per request, so the shell and the home page (which sends a member without reports on
// to their first section) share one call.
export const loadOrgMe = cache(async (orgId: string) => {
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
});

// The side menu inside an organization. An item shows when the member's ability (from
// getOrgMe's capabilities) grants its permission, on every record or on some ("conditional").
// Never by role name: a custom role or a property limit changes what shows without touching this
// list. Hiding is visibility only; the API checks every request.
//
// Field work comes first (board HP-Tab-Karyawan, draft 6): a member who does field work sees
// Tagihan hari ini, Kunjungan hari ini and Setor tunai before the office sections. An admin or
// Supervisor holds none of their actions (they read visits, visit:read, but never record one), so
// their menu starts at Beranda as before.
export const ORG_MENU: readonly OrgMenuItem[] = [
  {
    // "Tagihan hari ini": cash:record holds on the tenants the karyawan collects from only, so it
    // is "conditional" and shows the item.
    key: "field",
    label: "Tagihan hari ini",
    tabLabel: "Tagihan",
    segment: "field",
    permission: ["record", "cash"],
  },
  {
    // "Kunjungan hari ini" (board HP-Kunjungan): for a member who records visit outcomes, as the
    // tab board says. Not visit:read, which an admin and the Supervisor hold for reading the
    // outcomes: their list here would be empty, and their menu stays as it was. visit:record
    // holds on the karyawan's own tenants only, so it is "conditional" and shows the item.
    // The phone tab carries the count of visits still open ("3" on the board): PhoneTabBar
    // renders this item as PhoneVisitsTabLink, which reads openTotal from listFieldVisits. The
    // side menu on iPad and desktop has no count (the board draws only the phone).
    key: "visits",
    label: "Kunjungan hari ini",
    tabLabel: "Kunjungan",
    segment: "field/visits",
    permission: ["record", "visit"],
  },
  {
    // The karyawan's own cash, "Setor tunai": cash:deposit holds on their own records only, so
    // it is "conditional" and shows the item.
    key: "deposit",
    label: "Setor tunai",
    tabLabel: "Setor",
    segment: "field/deposit",
    permission: ["deposit", "cash"],
  },
  // Beranda is the organization's money summary: only for a member who may read reports.
  {
    key: "home",
    label: "Beranda",
    segment: "",
    permission: ["read", "report"],
  },
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

// Sections whose link is current on its own page only: the home, and Tagihan hari ini, whose
// path starts Setor tunai's.
const EXACT_SECTIONS: ReadonlySet<OrgMenuItem["key"]> = new Set([
  "home",
  "field",
]);

export function isExactSection(key: OrgMenuItem["key"]): boolean {
  return EXACT_SECTIONS.has(key);
}

// Where /o/<orgId> sends a member who may not read reports: the first section they may open (for
// a karyawan, Tagihan hari ini), so signing in or picking the organization lands there. Null when
// Beranda is theirs, or when they may open nothing (the home stays, with its short line).
export function landingSegment(ability: AppAbility): string | null {
  const visible = visibleOrgMenu(ability);
  if (visible.some((item) => item.key === "home")) return null;
  return visible[0]?.segment ?? null;
}

// The roles under the organization's name in the menu ("Admin", "Karyawan, Admin"). Display
// only: what the member may do comes from capabilities, never from these names. A role held
// for several properties shows once.
export function roleSummary(roles: readonly RoleRef[]): string {
  return [...new Set(roles.map((role) => role.name))].join(", ");
}

// What a screen's placeholder says to screen readers while its data streams in.
export const SCREEN_LOADING_LABEL = "Memuat…";

// The phone's tab bar (boards HP-Tab, approved 2026-11-08, and HP-Tab-Karyawan, draft 6): the
// sections the member may open in this order, at most four, then "Lainnya" with the rest. Field
// work first (Tagihan, Kunjungan, Setor), then Beranda, Penyewa, Penagihan and Properti. iPad and
// desktop keep the side menu.
export const PHONE_TAB_ORDER: readonly OrgMenuItem["key"][] = [
  "field",
  "visits",
  "deposit",
  "home",
  "tenants",
  "billing",
  "properties",
];
export const PHONE_TAB_COUNT = 4;
export const MORE_MENU_LABEL = "Lainnya";
export const SWITCH_ORG_LABEL = "Ganti usaha";

// The sections the tab bar shows, in its order, from the sections the member may open.
export function phoneTabs<T extends { key: OrgMenuItem["key"] }>(
  items: readonly T[],
): T[] {
  return PHONE_TAB_ORDER.flatMap((key) =>
    items.filter((item) => item.key === key),
  ).slice(0, PHONE_TAB_COUNT);
}

// The sections that live in "Lainnya" on the phone (every one that is not a tab), in the side
// menu's order.
export function moreMenuSections<T extends { key: OrgMenuItem["key"] }>(
  items: readonly T[],
): T[] {
  const tabs = new Set(phoneTabs(items).map((item) => item.key));
  return items.filter((item) => !tabs.has(item.key));
}

// Under the member's name in "Lainnya": "Admin di Griya Sejahtera".
export function moreMenuRoleLine(roles: string, organizationName: string) {
  return roles === "" ? organizationName : `${roles} di ${organizationName}`;
}
