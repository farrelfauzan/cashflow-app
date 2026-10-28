"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { toFormError } from "@/lib/api/form-error";
import { useGetOrgMe } from "@/lib/api/generated/members/members";
import { useListProperties } from "@/lib/api/generated/properties/properties";
import type { AppAbility } from "@/lib/capabilities";
import {
  LOADING_TEXT,
  ONLY_ASSIGNED_NOTE,
  READ_ONLY_NOTE,
  propertiesReadOnly,
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

// The properties screen: the list, and the panel that adds or changes one. What shows comes from
// the signed-in member's capabilities (getOrgMe); after every change the list query is
// invalidated and loads again from the API, occupancy included.
export function PropertiesView({
  orgId,
  organizationName,
}: PropertiesViewProps) {
  const orgMe = useGetOrgMe(orgId);
  const properties = useListProperties(orgId, { pageSize: 100 });
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

      {orgMe.data !== undefined && (
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
      ) : orgMe.data === undefined || properties.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : properties.data.properties.length === 0 ? (
        <p className="text-base text-muted-foreground">
          Belum ada properti di usaha ini.
          {permissions.canAddProperty && " Mulai dengan “Tambah properti”."}
        </p>
      ) : (
        <PropertyList
          orgId={orgId}
          properties={properties.data.properties}
          canEdit={permissions.canEditProperty}
          onEdit={(propertyId) => openPanel({ kind: "edit", propertyId })}
        />
      )}

      <PropertyPanel
        orgId={orgId}
        organizationName={organizationName}
        panel={panel}
        properties={properties.data?.properties ?? []}
        onPanelChange={setPanel}
        onNotice={setNotice}
      />
    </div>
  );
}
