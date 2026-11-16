export type OrgMenuItem = {
  key:
    | "field"
    | "deposit"
    | "home"
    | "properties"
    | "tenants"
    | "billing"
    | "members";
  // The side menu's name ("Tagihan hari ini").
  label: string;
  // The phone tab's shorter name ("Tagihan"); the label when absent.
  tabLabel?: string;
  // Below /o/<orgId>; "" is the organization's home.
  segment: string;
  // CASL's [verb, resource] for the API's "resource:verb"; null: every member sees it.
  permission: readonly [verb: string, resource: string] | null;
};

// One section in the org menu, as OrgNav and the tab bar render it.
export type OrgNavItem = {
  key: OrgMenuItem["key"];
  label: string;
  // The phone tab's name: "Tagihan" for "Tagihan hari ini".
  tabLabel: string;
  href: string;
};
