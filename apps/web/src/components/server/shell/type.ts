export type OrgMenuItem = {
  key: "home" | "properties" | "tenants" | "members";
  label: string;
  // Below /o/<orgId>; "" is the organization's home.
  segment: string;
  // CASL's [verb, resource] for the API's "resource:verb"; null: every member sees it.
  permission: readonly [verb: string, resource: string] | null;
};

// One section in the org menu, as OrgNav renders it.
export type OrgNavItem = {
  key: OrgMenuItem["key"];
  label: string;
  href: string;
};
