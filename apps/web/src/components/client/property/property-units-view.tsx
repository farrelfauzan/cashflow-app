"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { toFormError } from "@/lib/api/form-error";
import { useGetOrgMe } from "@/lib/api/generated/members/members";
import {
  useListProperties,
  useListUnits,
} from "@/lib/api/generated/properties/properties";
import type { AppAbility } from "@/lib/capabilities";
import {
  LOADING_TEXT,
  READ_ONLY_NOTE,
  propertyPermissions,
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

// One property's page: its units, and the panels that change the property or a unit. The
// property comes from the list of properties the member may see (there is no request for one);
// its units load once it is found.
export function PropertyUnitsView({
  orgId,
  propertyId,
  organizationName,
}: PropertyUnitsViewProps) {
  const orgMe = useGetOrgMe(orgId);
  const properties = useListProperties(orgId, { pageSize: 100 });
  const property = properties.data?.properties.find(
    (candidate) => candidate.id === propertyId,
  );
  const units = useListUnits(
    orgId,
    propertyId,
    { pageSize: 100 },
    {
      query: { enabled: property !== undefined },
    },
  );
  const [propertyPanel, setPropertyPanel] = useState<PropertyPanelState>({
    kind: "closed",
  });
  const [unitPanel, setUnitPanel] = useState<UnitPanelState>({
    kind: "closed",
  });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");

  const permissions = propertyPermissions(useAbility<AppAbility>());
  const failed = orgMe.isError ? orgMe.error : properties.error;

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
          void properties.refetch();
        }}
      />
    );
  }
  if (orgMe.data === undefined || properties.data === undefined) {
    return <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>;
  }
  if (property === undefined) return <PropertyNotFound orgId={orgId} />;

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
      ) : units.data.units.length === 0 ? (
        <p className="text-base text-muted-foreground">
          Belum ada unit di properti ini.
          {permissions.canManageUnits && " Mulai dengan “Tambah unit”."}
        </p>
      ) : (
        <UnitList
          units={units.data.units}
          canManage={permissions.canManageUnits}
          onEdit={(unitId) => openUnitPanel({ kind: "edit", unitId })}
        />
      )}

      <PropertyPanel
        orgId={orgId}
        organizationName={organizationName}
        panel={propertyPanel}
        properties={properties.data.properties}
        onPanelChange={setPropertyPanel}
        onNotice={setNotice}
      />
      <UnitPanel
        orgId={orgId}
        propertyId={property.id}
        propertyName={property.name}
        panel={unitPanel}
        units={units.data?.units ?? []}
        onPanelChange={setUnitPanel}
        onNotice={setNotice}
      />
    </div>
  );
}
