"use client";

import type { FilterOption, TenantFilters as Filters } from "./type";
import {
  ALL_PROPERTIES,
  ALL_PROPERTIES_LABEL,
  ALL_STATUSES_LABEL,
  STATUS_FILTER_OPTIONS,
  toStatusFilter,
} from "./utils";

import { FilterSelect } from "./filter-select";
import { FlaggedFilterChip } from "./flagged-filter-chip";
import { TenantSearchField } from "./tenant-search-field";

type TenantFiltersProps = {
  filters: Filters;
  properties: readonly FilterOption[];
  onChange: (filters: Filters) => void;
};

// Search, "Semua properti" and "Semua status" (and the flagged filter when it is on). They run in
// the browser: the list is not paginated.
export function TenantFilters({
  filters,
  properties,
  onChange,
}: TenantFiltersProps) {
  return (
    <div className="flex flex-col gap-2.5">
      <div className="flex flex-col gap-2.5 lg:flex-row lg:items-center lg:gap-3">
        <div className="lg:min-w-0 lg:flex-1">
          <TenantSearchField
            value={filters.search}
            onChange={(search) => onChange({ ...filters, search })}
          />
        </div>
        <div className="grid grid-cols-2 gap-2.5 lg:flex lg:gap-3">
          <FilterSelect
            label="Filter properti"
            value={filters.propertyId}
            allValue={ALL_PROPERTIES}
            allLabel={ALL_PROPERTIES_LABEL}
            options={properties}
            onValueChange={(propertyId) => onChange({ ...filters, propertyId })}
            className="lg:w-[210px]"
          />
          <FilterSelect
            label="Filter status"
            value={filters.status}
            allValue="all"
            allLabel={ALL_STATUSES_LABEL}
            options={STATUS_FILTER_OPTIONS}
            onValueChange={(value) =>
              onChange({ ...filters, status: toStatusFilter(value) })
            }
            className="lg:w-[190px]"
          />
        </div>
      </div>
      {filters.flaggedOnly && (
        <FlaggedFilterChip
          onClear={() => onChange({ ...filters, flaggedOnly: false })}
        />
      )}
    </div>
  );
}
