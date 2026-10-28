import type { OrgMenuItem } from "@/lib/organization/org-menu";

// One section in the org menu, as OrgNav renders it.
export type OrgNavItem = {
  key: OrgMenuItem["key"];
  label: string;
  href: string;
};
