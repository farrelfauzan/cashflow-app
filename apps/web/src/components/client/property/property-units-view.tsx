"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useState } from "react";

import { ListPagination } from "@/components/client/list/list-pagination";
import { ListSortSelect } from "@/components/client/list/list-sort-select";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
  readSort,
} from "@/components/client/list/utils";
import { toFormError } from "@/lib/api/form-error";
import { useGetOrgMe } from "@/lib/api/generated/members/members";
import {
  useGetProperty,
  useListUnits,
} from "@/lib/api/generated/properties/properties";
import type { AppAbility } from "@/lib/capabilities";
import {
  LOADING_TEXT,
  READ_ONLY_NOTE,
  isPropertyMissing,
  propertyPermissions,
  UNIT_SORT_OPTIONS,
  unitsReadOnly,
} from "./utils";
import type { PropertyPanelState, UnitPanelState } from "./type";

import { PropertyLoadError } from "./property-load-error";
import { PropertyNotFound } from "./property-not-found";
import { PropertyPanel } from "./property-panel";
import { PropertyUnitsHeader } from "./property-units-header";
import { UnitList } from "./unit-list";
import { UnitPanel } from "./unit-panel";

type PropertyUnitsViewProps = {
  orgId: string;
  propertyId: string;
  organizationName: string;
};

// One property's page: one page of its units (page, size and sort in the URL), and the panels that
// change the property or a unit. The property comes from getProperty; its units load once it is
// found.
export function PropertyUnitsView({
  orgId,
  propertyId,
  organizationName,
}: PropertyUnitsViewProps) {
  const orgMe = useGetOrgMe(orgId);
  const url = useListUrl();
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);
  const sort = readSort(url.searchParams, UNIT_SORT_OPTIONS);
  const found = useGetProperty(orgId, propertyId);
  const property = found.data;
  const units = useListUnits(
    orgId,
    propertyId,
    { page, pageSize, sort },
    {
      query: {
        enabled: property !== undefined,
        placeholderData: keepPreviousData,
      },
    },
  );
  useLastPageRedirect(url, page, pageSize, units.data);
  const [propertyPanel, setPropertyPanel] = useState<PropertyPanelState>({
    kind: "closed",
  });
  const [unitPanel, setUnitPanel] = useState<UnitPanelState>({
    kind: "closed",
  });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");

  const permissions = propertyPermissions(useAbility<AppAbility>());
  const missing = isPropertyMissing(found.error);
  const failed = orgMe.isError ? orgMe.error : missing ? null : found.error;

  function openPropertyPanel(next: PropertyPanelState) {
    setNotice("");
    setPropertyPanel(next);
  }

  function openUnitPanel(next: UnitPanelState) {
    setNotice("");
    setUnitPanel(next);
  }

  if (failed !== null) {
    return (
      <PropertyLoadError
        message={toFormError(failed, []).message}
        onRetry={() => {
          void orgMe.refetch();
          void found.refetch();
        }}
      />
    );
  }
  if (missing) return <PropertyNotFound orgId={orgId} />;
  if (orgMe.data === undefined || property === undefined) {
    return <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>;
  }

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <PropertyUnitsHeader
        orgId={orgId}
        property={property}
        canEditProperty={permissions.canEditProperty}
        canAddUnit={permissions.canManageUnits}
        onEditProperty={() =>
          openPropertyPanel({ kind: "edit", propertyId: property.id })
        }
        onAddUnit={() => openUnitPanel({ kind: "add" })}
      />

      {unitsReadOnly(permissions) && (
        <p className="text-[15px] leading-normal font-semibold text-foreground/80 md:text-base">
          {READ_ONLY_NOTE}
        </p>
      )}

      <p
        role="status"
        aria-live="polite"
        className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
      >
        {notice}
      </p>

      {units.isError ? (
        <PropertyLoadError
          message={toFormError(units.error, []).message}
          onRetry={() => void units.refetch()}
        />
      ) : units.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : units.data.total === 0 ? (
        <p className="text-base text-muted-foreground">
          Belum ada unit di properti ini.
          {permissions.canManageUnits && " Mulai dengan “Tambah unit”."}
        </p>
      ) : (
        <>
          <ListSortSelect
            options={UNIT_SORT_OPTIONS}
            value={sort}
            onChange={url.change}
          />
          <div
            aria-busy={units.isPlaceholderData}
            className="flex flex-col gap-4 aria-busy:opacity-60"
          >
            <UnitList
              units={units.data.items}
              canManage={permissions.canManageUnits}
              onEdit={(unitId) => openUnitPanel({ kind: "edit", unitId })}
            />
            <ListPagination
              page={page}
              pageSize={pageSize}
              total={units.data.total}
              hrefFor={url.hrefFor}
              onPageSizeChange={(size) =>
                url.change({ [PAGE_SIZE_PARAM]: size })
              }
            />
          </div>
        </>
      )}

      <PropertyPanel
        orgId={orgId}
        organizationName={organizationName}
        panel={propertyPanel}
        properties={[property]}
        onPanelChange={setPropertyPanel}
        onNotice={setNotice}
      />
      <UnitPanel
        orgId={orgId}
        propertyId={property.id}
        propertyName={property.name}
        propertyKind={property.kind}
        panel={unitPanel}
        units={units.data?.items ?? []}
        onPanelChange={setUnitPanel}
        onNotice={setNotice}
      />
    </div>
  );
}
