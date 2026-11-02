"use client";

import { ListSearchField } from "@/components/client/list/list-search-field";
import { ListSortSelect } from "@/components/client/list/list-sort-select";
import type { ListUrl } from "@/components/client/list/type";
import { SEARCH_PARAM } from "@/components/client/list/utils";
import { FilterSelect } from "@/components/client/tenant/filter-select";
import type { FilterOption } from "@/components/client/tenant/type";
import {
  ALL_PROPERTIES,
  ALL_PROPERTIES_LABEL,
  ALL_STATUSES,
  ALL_STATUSES_LABEL,
  filterParam,
  PROPERTY_PARAM,
  STATUS_PARAM,
} from "@/components/client/tenant/utils";
import { cn } from "@/lib/utils";
import type { ListInvoicesSort } from "@/lib/api/generated/model";
import type { InvoiceListFilters } from "./type";
import {
  INVOICE_SORT_OPTIONS,
  SEARCH_LABEL,
  STATUS_FILTER_OPTIONS,
} from "./utils";

type InvoiceFiltersProps = {
  // The search as typed, from the URL.
  search: string;
  filters: InvoiceListFilters;
  sort: ListInvoicesSort;
  // The property filter's choices; null hides it (the member cannot read properties, or they
  // did not load).
  properties: readonly FilterOption[] | null;
  onChange: ListUrl["change"];
};

// Search, "Status" and "Properti", then "Urutkan" on its own line. Each change goes into the URL
// (and back to page 1) and the API narrows or orders the list.
export function InvoiceFilters({
  search,
  filters,
  sort,
  properties,
  onChange,
}: InvoiceFiltersProps) {
  return (
    <div className="flex flex-col gap-2.5">
      <div className="flex flex-col gap-2.5 lg:flex-row lg:items-center lg:gap-3">
        <div className="lg:min-w-0 lg:flex-1">
          <ListSearchField
            value={search}
            label={SEARCH_LABEL}
            onSearch={(q) => onChange({ [SEARCH_PARAM]: q }, { replace: true })}
          />
        </div>
        <div
          className={cn(
            "grid gap-2.5 lg:flex lg:gap-3",
            properties === null ? "grid-cols-1" : "grid-cols-2",
          )}
        >
          <FilterSelect
            label="Status"
            value={filters.status ?? ALL_STATUSES}
            allValue={ALL_STATUSES}
            allLabel={ALL_STATUSES_LABEL}
            options={STATUS_FILTER_OPTIONS}
            onValueChange={(value) =>
              onChange({ [STATUS_PARAM]: filterParam(value, ALL_STATUSES) })
            }
            className="lg:w-[190px]"
          />
          {properties !== null && (
            <FilterSelect
              label="Properti"
              value={filters.propertyId ?? ALL_PROPERTIES}
              allValue={ALL_PROPERTIES}
              allLabel={ALL_PROPERTIES_LABEL}
              options={properties}
              onValueChange={(value) =>
                onChange({
                  [PROPERTY_PARAM]: filterParam(value, ALL_PROPERTIES),
                })
              }
              className="lg:w-[210px]"
            />
          )}
        </div>
      </div>
      <ListSortSelect
        options={INVOICE_SORT_OPTIONS}
        value={sort}
        onChange={onChange}
      />
    </div>
  );
}
