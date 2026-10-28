export type OrgMenuItem = {
  key: "home" | "properties" | "tenants" | "members";
  label: string;
  // Below /o/<orgId>; "" is the organization's home.
  segment: string;
  // null: every member sees it.
  action: string | null;
};

// One section in the org menu, as OrgNav renders it.
export type OrgNavItem = {
  key: OrgMenuItem["key"];
  label: string;
  href: string;
};
