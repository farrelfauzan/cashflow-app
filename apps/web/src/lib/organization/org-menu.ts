import type { Capability } from "@/lib/api/generated/model";

// The side menu inside an organization. An item shows when the member's capabilities from
// getOrgMe allow its action, or allow it on some records ("conditional"). Never by role name:
// a custom role or a property limit changes what shows without touching this list. Hiding is
// visibility only; the API checks every request.

export type OrgMenuItem = {
  key: "home" | "properties" | "tenants" | "members";
  label: string;
  // Below /o/<orgId>; "" is the organization's home.
  segment: string;
  // null: every member sees it.
  action: string | null;
};

export const ORG_MENU: readonly OrgMenuItem[] = [
  { key: "home", label: "Beranda", segment: "", action: null },
  {
    key: "properties",
    label: "Properti",
    segment: "properties",
    action: "property:read",
  },
  {
    key: "tenants",
    label: "Penyewa",
    segment: "tenants",
    action: "tenant:read",
  },
  {
    key: "members",
    label: "Anggota tim",
    segment: "members",
    action: "member:read",
  },
];

// The path of an organization page: orgPath(id) is its home, orgPath(id, "members") a section.
export function orgPath(orgId: string, segment = ""): string {
  const base = `/o/${encodeURIComponent(orgId)}`;
  return segment === "" ? base : `${base}/${segment}`;
}

// True when the member may do `action` at least on some records. An action the API does not
// list is denied, like on the server.
export function canSee(
  capabilities: readonly Capability[],
  action: string,
): boolean {
  const capability = capabilities.find((c) => c.action === action);
  return capability?.access === "allow" || capability?.access === "conditional";
}

export function visibleOrgMenu(
  capabilities: readonly Capability[],
): OrgMenuItem[] {
  return ORG_MENU.filter(
    (item) => item.action === null || canSee(capabilities, item.action),
  );
}
