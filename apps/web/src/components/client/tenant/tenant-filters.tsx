"use client";

import { ListSearchField } from "@/components/client/list/list-search-field";
import { ListSortSelect } from "@/components/client/list/list-sort-select";
import type { AddedSort, ListUrl } from "@/components/client/list/type";
import {
  ADDED_SORT_OPTIONS,
  SEARCH_PARAM,
} from "@/components/client/list/utils";
import { cn } from "@/lib/utils";
import type { FilterOption, TenantListFilters } from "./type";
import {
  ALL_PROPERTIES,
  ALL_PROPERTIES_LABEL,
  ALL_STATUSES,
  ALL_STATUSES_LABEL,
  COLLECTOR_DISABLED_PARAM,
  filterParam,
  PROPERTY_PARAM,
  SEARCH_LABEL,
  STATUS_FILTER_OPTIONS,
  STATUS_PARAM,
} from "./utils";

import { FilterSelect } from "./filter-select";
import { FlaggedFilterChip } from "./flagged-filter-chip";

type TenantFiltersProps = {
  // The search as typed, from the URL.
  search: string;
  filters: TenantListFilters;
  // The sort, from the URL.
  sort: AddedSort;
  // The property filter's choices; null hides it (the member cannot read properties, or they
  // did not load).
  properties: readonly FilterOption[] | null;
  onChange: ListUrl["change"];
};

// Search, "Semua properti" and "Semua status", then "Urutkan" on its own line (the search would be
// squeezed on iPad if it shared the row), and the flagged filter when it is on. Each change goes
// into the URL and the API narrows or orders the list.
export function TenantFilters({
  search,
  filters,
  sort,
  properties,
  onChange,
}: TenantFiltersProps) {
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
          {properties !== null && (
            <FilterSelect
              label="Filter properti"
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
          <FilterSelect
            label="Filter status"
            value={filters.status ?? ALL_STATUSES}
            allValue={ALL_STATUSES}
            allLabel={ALL_STATUSES_LABEL}
            options={STATUS_FILTER_OPTIONS}
            onValueChange={(value) =>
              onChange({ [STATUS_PARAM]: filterParam(value, ALL_STATUSES) })
            }
            className="lg:w-[190px]"
          />
        </div>
      </div>
      <ListSortSelect
        options={ADDED_SORT_OPTIONS}
        value={sort}
        onChange={onChange}
      />
      {filters.collectorDisabled === true && (
        <FlaggedFilterChip
          onClear={() => onChange({ [COLLECTOR_DISABLED_PARAM]: null })}
        />
      )}
    </div>
  );
}
