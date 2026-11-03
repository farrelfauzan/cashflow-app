"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useMemo } from "react";

import { ListPagination } from "@/components/client/list/list-pagination";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
} from "@/components/client/list/utils";
import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useListMissingMeterReadings } from "@/lib/api/generated/money/money";
import type { AppAbility } from "@/lib/capabilities";
import {
  LOADING_TEXT,
  METER_ALL_RECORDED_TEXT,
  METER_PARAM,
  meterShowingText,
  missingMeterRow,
} from "./utils";

import { MeterFilterChip } from "./meter-filter-chip";
import { MeterMissingList } from "./meter-missing-list";

type MeterMissingViewProps = {
  orgId: string;
};

// "Meteran belum dicatat": the leases with a meter amount missing for last month (the API's
// default month), one page at a time, in place of the invoices. The chip takes the view off. The
// way to "Catat meteran" is the link to Detail penyewa on each row.
export function MeterMissingView({ orgId }: MeterMissingViewProps) {
  const ability = useAbility<AppAbility>();
  const canReadTenants = ability.can("read", "tenant");

  const url = useListUrl();
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);

  const missing = useListMissingMeterReadings(
    orgId,
    { page, pageSize },
    { query: { placeholderData: keepPreviousData, refetchOnMount: "always" } },
  );
  useLastPageRedirect(url, page, pageSize, missing.data);

  const items = missing.data?.items;
  const rows = useMemo(
    () => (items === undefined ? [] : items.map(missingMeterRow)),
    [items],
  );

  return (
    <div className="flex flex-col gap-4">
      <MeterFilterChip onClear={() => url.change({ [METER_PARAM]: null })} />
      {missing.error !== null ? (
        <TenantLoadError
          message={toFormError(missing.error, []).message}
          onRetry={() => void missing.refetch()}
        />
      ) : missing.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : missing.data.total === 0 ? (
        <p role="status" className="text-base">
          {METER_ALL_RECORDED_TEXT}
        </p>
      ) : (
        <div
          aria-busy={missing.isPlaceholderData}
          className="flex flex-col gap-4 aria-busy:opacity-60"
        >
          <p className="text-base text-muted-foreground">
            {meterShowingText(missing.data.total)}
          </p>
          <MeterMissingList
            orgId={orgId}
            rows={rows}
            canReadTenants={canReadTenants}
          />
          <ListPagination
            page={page}
            pageSize={pageSize}
            total={missing.data.total}
            hrefFor={url.hrefFor}
            onPageSizeChange={(size) => url.change({ [PAGE_SIZE_PARAM]: size })}
          />
        </div>
      )}
    </div>
  );
}
