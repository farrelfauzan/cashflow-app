"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { ListPagination } from "@/components/client/list/list-pagination";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  ADDED_SORT_OPTIONS,
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
  readSearch,
  readSort,
} from "@/components/client/list/utils";
import { toFormError } from "@/lib/api/form-error";
import { useGetOrgMe } from "@/lib/api/generated/members/members";
import { useListProperties } from "@/lib/api/generated/properties/properties";
import { useListTenants } from "@/lib/api/generated/tenants/tenants";
import type { AppAbility } from "@/lib/capabilities";
import { orgPath } from "@/lib/org-path";
import { jakartaToday } from "@/lib/format/date";
import {
  CLEAR_FILTERS,
  COLLECTOR_DISABLED_PARAM,
  isFiltering,
  LOADING_TEXT,
  propertyFilterOptions,
  readTenantFilters,
  tenantPermissions,
  tenantRow,
} from "./utils";
import type { TenantPanelState } from "./type";

import { TenantFilters } from "./tenant-filters";
import { TenantFlagBanner } from "./tenant-flag-banner";
import { TenantList } from "./tenant-list";
import { TenantLoadError } from "./tenant-load-error";
import { TenantPanel } from "./tenant-panel";
import { TenantsEmpty } from "./tenants-empty";
import { TenantsHeader } from "./tenants-header";
import { TenantsNoResults } from "./tenants-no-results";

type TenantsViewProps = {
  orgId: string;
  organizationName: string;
};

// The tenant list: one page of the tenants the member may see, with the lease they live in. The
// page, size, search, filters and sort live in the URL and the API applies them. What shows beyond
// that (the banner, the property filter, "Tambah penyewa") comes from the member's capabilities
// (getOrgMe).
export function TenantsView({ orgId, organizationName }: TenantsViewProps) {
  const orgMe = useGetOrgMe(orgId);
  const ability = useAbility<AppAbility>();
  const permissions = tenantPermissions(ability);

  const url = useListUrl();
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);
  const search = readSearch(url.searchParams);
  const filters = readTenantFilters(url.searchParams);
  const sort = readSort(url.searchParams, ADDED_SORT_OPTIONS);
  const tenants = useListTenants(
    orgId,
    { page, pageSize, sort, ...filters },
    { query: { placeholderData: keepPreviousData } },
  );
  useLastPageRedirect(url, page, pageSize, tenants.data);

  // The banner's count: every flagged tenant, whatever the list is narrowed to.
  const flagged = useListTenants(
    orgId,
    { collectorDisabled: true, pageSize: 1 },
    { query: { enabled: permissions.canManageLeases } },
  );
  const flaggedTotal = flagged.data?.total ?? 0;

  // The property filter's choices. Hidden when the member cannot read properties or they fail.
  const canReadProperties = ability.can("read", "property");
  const properties = useListProperties(
    orgId,
    { pageSize: 100 },
    { query: { enabled: canReadProperties } },
  );
  const propertyItems = properties.data?.items;
  const propertyOptions = useMemo(
    () =>
      !canReadProperties || propertyItems === undefined
        ? null
        : propertyFilterOptions(propertyItems),
    [canReadProperties, propertyItems],
  );

  // "Today" in Jakarta, read once when the screen opens; the utils take it as an argument.
  const [today] = useState(() => jakartaToday(new Date()));
  const items = tenants.data?.items;
  const rows = useMemo(
    () => (items === undefined ? [] : items.map((t) => tenantRow(t, today))),
    [items, today],
  );

  const failed = orgMe.isError ? orgMe.error : tenants.error;
  const empty =
    tenants.data !== undefined &&
    tenants.data.total === 0 &&
    !isFiltering(filters);

  const [panel, setPanel] = useState<TenantPanelState>({ kind: "closed" });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");
  const addTenant = () => {
    setNotice("");
    setPanel({ kind: "add" });
  };

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <TenantsHeader
        organizationName={organizationName}
        canAdd={permissions.canAddTenant && !empty}
        onAdd={addTenant}
        importHref={
          permissions.canImport ? orgPath(orgId, "tenants/import") : null
        }
      />

      <p
        role="status"
        aria-live="polite"
        className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
      >
        {notice}
      </p>

      {failed !== null ? (
        <TenantLoadError
          message={toFormError(failed, []).message}
          onRetry={() => {
            void orgMe.refetch();
            void tenants.refetch();
          }}
        />
      ) : orgMe.data === undefined || tenants.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : empty ? (
        <TenantsEmpty canAdd={permissions.canAddTenant} onAdd={addTenant} />
      ) : (
        <>
          {permissions.canManageLeases && flaggedTotal > 0 && (
            <TenantFlagBanner
              count={flaggedTotal}
              showing={filters.collectorDisabled === true}
              onShow={() => url.change({ [COLLECTOR_DISABLED_PARAM]: "true" })}
            />
          )}
          <TenantFilters
            search={search}
            filters={filters}
            sort={sort}
            properties={propertyOptions}
            onChange={url.change}
          />
          <div
            aria-busy={tenants.isPlaceholderData}
            className="flex flex-col gap-4 aria-busy:opacity-60"
          >
            {tenants.data.total === 0 ? (
              <TenantsNoResults onReset={() => url.change(CLEAR_FILTERS)} />
            ) : (
              <>
                <TenantList orgId={orgId} rows={rows} />
                <ListPagination
                  page={page}
                  pageSize={pageSize}
                  total={tenants.data.total}
                  hrefFor={url.hrefFor}
                  onPageSizeChange={(size) =>
                    url.change({ [PAGE_SIZE_PARAM]: size })
                  }
                />
              </>
            )}
          </div>
        </>
      )}

      <TenantPanel
        orgId={orgId}
        organizationName={organizationName}
        today={today}
        canManageLeases={permissions.canManageLeases}
        canAddChargeType={permissions.canAddChargeType}
        panel={panel}
        onPanelChange={setPanel}
        onNotice={setNotice}
      />
    </div>
  );
}
