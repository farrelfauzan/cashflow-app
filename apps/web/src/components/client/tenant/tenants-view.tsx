"use client";

import { useMemo, useState } from "react";

import { toFormError } from "@/lib/api/form-error";
import { useGetOrgMe } from "@/lib/api/generated/members/members";
import { useListTenants } from "@/lib/api/generated/tenants/tenants";
import { jakartaToday } from "@/lib/format/date";
import type { TenantFilters as Filters } from "./type";
import {
  filterTenants,
  flaggedCount,
  LOADING_TEXT,
  NO_FILTERS,
  NO_PERMISSIONS,
  propertyOptions,
  tenantPermissions,
  tenantRow,
} from "./utils";

import { TenantFilters } from "./tenant-filters";
import { TenantFlagBanner } from "./tenant-flag-banner";
import { TenantList } from "./tenant-list";
import { TenantLoadError } from "./tenant-load-error";
import { TenantsEmpty } from "./tenants-empty";
import { TenantsHeader } from "./tenants-header";
import { TenantsNoResults } from "./tenants-no-results";

type TenantsViewProps = {
  orgId: string;
  organizationName: string;
};

// The tenant list: every tenant the member may see, with the lease they live in. The list is not
// paginated, so search, filters and the flag count run here in the browser. Read-only: what shows
// beyond that (the banner) comes from the member's capabilities (getOrgMe).
export function TenantsView({ orgId, organizationName }: TenantsViewProps) {
  const orgMe = useGetOrgMe(orgId);
  const tenants = useListTenants(orgId, { pageSize: 100 });
  const [filters, setFilters] = useState<Filters>(NO_FILTERS);
  // "Today" in Jakarta, read once when the screen opens; the utils take it as an argument.
  const [today] = useState(() => jakartaToday(new Date()));

  const all = tenants.data?.tenants;
  const rows = useMemo(
    () =>
      all === undefined
        ? []
        : filterTenants(all, filters, today).map((t) => tenantRow(t, today)),
    [all, filters, today],
  );
  const properties = useMemo(() => propertyOptions(all ?? []), [all]);

  const permissions =
    orgMe.data === undefined
      ? NO_PERMISSIONS
      : tenantPermissions(orgMe.data.capabilities);
  const failed = orgMe.isError ? orgMe.error : tenants.error;
  const flagged = all === undefined ? 0 : flaggedCount(all);

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <TenantsHeader organizationName={organizationName} />

      {failed !== null ? (
        <TenantLoadError
          message={toFormError(failed, []).message}
          onRetry={() => {
            void orgMe.refetch();
            void tenants.refetch();
          }}
        />
      ) : orgMe.data === undefined || all === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : all.length === 0 ? (
        <TenantsEmpty />
      ) : (
        <>
          {permissions.canManageLeases && flagged > 0 && (
            <TenantFlagBanner
              count={flagged}
              showing={filters.flaggedOnly}
              onShow={() => setFilters({ ...filters, flaggedOnly: true })}
            />
          )}
          <TenantFilters
            filters={filters}
            properties={properties}
            onChange={setFilters}
          />
          {rows.length === 0 ? (
            <TenantsNoResults onReset={() => setFilters(NO_FILTERS)} />
          ) : (
            <TenantList orgId={orgId} rows={rows} />
          )}
        </>
      )}
    </div>
  );
}
