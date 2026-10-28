import {
  Building2,
  House,
  Users,
  UserCog,
  type LucideIcon,
} from "lucide-react";

import { ApiError } from "@/lib/api/errors";
import { getOrgMe } from "@/lib/api/generated/members/members";

import type { OrgNavItem } from "./type";

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
