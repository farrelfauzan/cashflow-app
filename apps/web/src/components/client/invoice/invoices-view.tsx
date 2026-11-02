"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { ListPagination } from "@/components/client/list/list-pagination";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
  readSearch,
  readSort,
} from "@/components/client/list/utils";
import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { propertyFilterOptions } from "@/components/client/tenant/utils";
import { toFormError } from "@/lib/api/form-error";
import { useListInvoices } from "@/lib/api/generated/money/money";
import { useListProperties } from "@/lib/api/generated/properties/properties";
import type { AppAbility } from "@/lib/capabilities";
import { jakartaToday } from "@/lib/format/date";
import {
  CLEAR_FILTERS,
  currentPeriod,
  FORBIDDEN_TEXT,
  INVOICE_SORT_OPTIONS,
  invoiceRow,
  isFiltering,
  isForbidden,
  isLatestMonth,
  listPeriod,
  LOADING_TEXT,
  monthLabel,
  nextMonth,
  noInvoicesText,
  PERIOD_PARAM,
  periodParam,
  previousMonth,
  readInvoiceFilters,
  shownPeriod,
  summaryTiles,
} from "./utils";

import { InvoiceFilters } from "./invoice-filters";
import { InvoiceList } from "./invoice-list";
import { InvoiceTiles } from "./invoice-tiles";
import { InvoicesEmpty } from "./invoices-empty";
import { InvoicesHeader } from "./invoices-header";
import { InvoicesNoResults } from "./invoices-no-results";

type InvoicesViewProps = {
  orgId: string;
};

// Penagihan: the month's invoices, one page at a time, with the three tiles of the whole month
// above them. The month, page, size, search, filters and sort live in the URL and the API applies
// them. Which controls show (the name's link, the property filter) comes from the member's
// capabilities (getOrgMe), never from a role name.
export function InvoicesView({ orgId }: InvoicesViewProps) {
  const ability = useAbility<AppAbility>();
  const canRead = ability.can("read", "invoice");
  const canReadTenants = ability.can("read", "tenant");

  const url = useListUrl();
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);
  const search = readSearch(url.searchParams);
  const filters = readInvoiceFilters(url.searchParams);
  const sort = readSort(url.searchParams, INVOICE_SORT_OPTIONS);

  // "Today" in Jakarta, read once when the screen opens; this month comes from it.
  const [today] = useState(() => jakartaToday(new Date()));
  const thisMonth = currentPeriod(today);
  const period = shownPeriod(filters.period, thisMonth);
  const allMonths = filters.status === "overdue";

  const invoices = useListInvoices(
    orgId,
    {
      period: listPeriod(filters),
      status: filters.status,
      propertyId: filters.propertyId,
      q: filters.q,
      sort,
      page,
      pageSize,
    },
    { query: { enabled: canRead, placeholderData: keepPreviousData } },
  );
  useLastPageRedirect(url, page, pageSize, invoices.data);

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

  const items = invoices.data?.items;
  const rows = useMemo(
    () => (items === undefined ? [] : items.map(invoiceRow)),
    [items],
  );

  const forbidden = !canRead || isForbidden(invoices.error);
  const empty =
    invoices.data !== undefined &&
    invoices.data.total === 0 &&
    !isFiltering(filters);

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <InvoicesHeader
        monthLabel={monthLabel(period)}
        allMonths={allMonths}
        canGoNext={!isLatestMonth(period, thisMonth)}
        onPrevious={() =>
          url.change({
            [PERIOD_PARAM]: periodParam(previousMonth(period), thisMonth),
          })
        }
        onNext={() =>
          url.change({
            [PERIOD_PARAM]: periodParam(nextMonth(period), thisMonth),
          })
        }
      />

      {forbidden ? (
        <p role="alert" className="text-base">
          {FORBIDDEN_TEXT}
        </p>
      ) : invoices.error !== null ? (
        <TenantLoadError
          message={toFormError(invoices.error, []).message}
          onRetry={() => void invoices.refetch()}
        />
      ) : invoices.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : (
        <>
          <InvoiceTiles
            tiles={summaryTiles(invoices.data.summary, period, thisMonth)}
          />
          {empty ? (
            <InvoicesEmpty text={noInvoicesText(period, thisMonth)} />
          ) : (
            <>
              <InvoiceFilters
                search={search}
                filters={filters}
                sort={sort}
                properties={propertyOptions}
                onChange={url.change}
              />
              <div
                aria-busy={invoices.isPlaceholderData}
                className="flex flex-col gap-4 aria-busy:opacity-60"
              >
                {invoices.data.total === 0 ? (
                  <InvoicesNoResults
                    onReset={() => url.change(CLEAR_FILTERS)}
                  />
                ) : (
                  <>
                    <InvoiceList
                      orgId={orgId}
                      rows={rows}
                      canReadTenants={canReadTenants}
                    />
                    <ListPagination
                      page={page}
                      pageSize={pageSize}
                      total={invoices.data.total}
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
        </>
      )}
    </div>
  );
}
