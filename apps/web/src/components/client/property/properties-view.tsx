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
import { useListProperties } from "@/lib/api/generated/properties/properties";
import type { AppAbility } from "@/lib/capabilities";
import { useHydrated } from "@/lib/use-hydrated";
import {
  LOADING_TEXT,
  ONLY_ASSIGNED_NOTE,
  READ_ONLY_NOTE,
  propertiesReadOnly,
  PROPERTY_SORT_OPTIONS,
  propertyPermissions,
} from "./utils";
import type { PropertyPanelState } from "./type";

import { PropertiesHeader } from "./properties-header";
import { PropertyList } from "./property-list";
import { PropertyLoadError } from "./property-load-error";
import { PropertyPanel } from "./property-panel";

type PropertiesViewProps = {
  orgId: string;
  organizationName: string;
};

// The properties screen: one page of the list (page, size and sort in the URL), and the panel that
// adds or changes one. What shows comes from the signed-in member's capabilities (getOrgMe);
// after every change the list query is invalidated and loads again from the API, occupancy
// included.
export function PropertiesView({
  orgId,
  organizationName,
}: PropertiesViewProps) {
  const orgMe = useGetOrgMe(orgId);
  // The shell loads getOrgMe too, and its answer can reach the shared cache before this view
  // hydrates. Until hydration is done, render as the server did (without it).
  const orgMeData = useHydrated() ? orgMe.data : undefined;
  const url = useListUrl();
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);
  const sort = readSort(url.searchParams, PROPERTY_SORT_OPTIONS);
  const properties = useListProperties(
    orgId,
    { page, pageSize, sort },
    { query: { placeholderData: keepPreviousData } },
  );
  useLastPageRedirect(url, page, pageSize, properties.data);
  const [panel, setPanel] = useState<PropertyPanelState>({ kind: "closed" });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");

  const permissions = propertyPermissions(useAbility<AppAbility>());
  const failed = orgMe.isError ? orgMe.error : properties.error;

  function openPanel(next: PropertyPanelState) {
    setNotice("");
    setPanel(next);
  }

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <PropertiesHeader
        organizationName={organizationName}
        canAdd={permissions.canAddProperty}
        onAdd={() => openPanel({ kind: "add" })}
      />

      {orgMeData !== undefined && (
        <div className="flex flex-col gap-1">
          {permissions.seesOnlyAssigned && (
            <p className="text-[15px] leading-normal font-semibold text-foreground/80 md:text-base">
              {ONLY_ASSIGNED_NOTE}
            </p>
          )}
          {propertiesReadOnly(permissions) && (
            <p className="text-[15px] leading-normal font-semibold text-foreground/80 md:text-base">
              {READ_ONLY_NOTE}
            </p>
          )}
        </div>
      )}

      <p
        role="status"
        aria-live="polite"
        className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
      >
        {notice}
      </p>

      {failed !== null ? (
        <PropertyLoadError
          message={toFormError(failed, []).message}
          onRetry={() => {
            void orgMe.refetch();
            void properties.refetch();
          }}
        />
      ) : orgMeData === undefined || properties.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : properties.data.total === 0 ? (
        <p className="text-base text-muted-foreground">
          Belum ada properti di usaha ini.
          {permissions.canAddProperty && " Mulai dengan “Tambah properti”."}
        </p>
      ) : (
        <>
          <ListSortSelect
            options={PROPERTY_SORT_OPTIONS}
            value={sort}
            onChange={url.change}
          />
          <div
            aria-busy={properties.isPlaceholderData}
            className="flex flex-col gap-4 aria-busy:opacity-60"
          >
            <PropertyList
              orgId={orgId}
              properties={properties.data.items}
              canEdit={permissions.canEditProperty}
              onEdit={(propertyId) => openPanel({ kind: "edit", propertyId })}
            />
            <ListPagination
              page={page}
              pageSize={pageSize}
              total={properties.data.total}
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
        panel={panel}
        properties={properties.data?.items ?? []}
        onPanelChange={setPanel}
        onNotice={setNotice}
      />
    </div>
  );
}
