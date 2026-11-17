"use client";

import type { ReactNode } from "react";

import { useListFieldVisits } from "@/lib/api/generated/field/field";

import { PhoneTabLink } from "./phone-tab-link";
import { visitsBadgeText, visitsTabAccessibleName } from "./utils";

type PhoneVisitsTabLinkProps = {
  orgId: string;
  href: string;
  label: string;
  // The section's icon, rendered on the server.
  icon: ReactNode;
};

// The Kunjungan tab with the count of visits still open today (board HP-Kunjungan). The count is
// openTotal of listFieldVisits, read in the browser so it follows a saved outcome, Terima tunai
// or Catat janji bayar without a reload: their invalidation of the visits list refreshes this
// query too (same key prefix). The item only exists for a member who may record visits, so the
// tab bar renders this for them alone. No badge while loading, on an error or when none are open.
export function PhoneVisitsTabLink({
  orgId,
  href,
  label,
  icon,
}: PhoneVisitsTabLinkProps) {
  const visits = useListFieldVisits(orgId, { pageSize: 1 });
  const openTotal = visits.isSuccess ? visits.data.openTotal : 0;

  return (
    <PhoneTabLink
      href={href}
      exact={false}
      label={label}
      icon={icon}
      badge={openTotal > 0 ? visitsBadgeText(openTotal) : undefined}
      accessibleName={
        openTotal > 0 ? visitsTabAccessibleName(label, openTotal) : undefined
      }
    />
  );
}
