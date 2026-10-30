import type {
  PropertyInput,
  PropertyKind,
  UnitInput,
} from "@/lib/api/generated/model";

export type PropertyPermissions = {
  // "Tambah properti": the API accepts it only with property:manage for the whole organization,
  // so a member limited to some properties ("conditional") does not see it.
  canAddProperty: boolean;
  // "Ubah" and "Ubah properti".
  canEditProperty: boolean;
  // "Tambah unit" and "Ubah" on a unit.
  canManageUnits: boolean;
  // property:read on some properties only: the list holds just the ones assigned to the member.
  seesOnlyAssigned: boolean;
};

// The side panels of the property screens; one at a time on each screen.

// "Tambah properti" and "Ubah properti" (shared by the list and a property's page).
export type PropertyPanelState =
  | { kind: "closed" }
  | { kind: "add" }
  // Only the id is kept: the property shown is always the one in the latest list.
  | { kind: "edit"; propertyId: string };

// "Tambah unit" and "Ubah unit".
export type UnitPanelState =
  { kind: "closed" } | { kind: "add" } | { kind: "edit"; unitId: string };

export type OpenPropertyPanel = Exclude<PropertyPanelState, { kind: "closed" }>;

export type OpenUnitPanel = Exclude<UnitPanelState, { kind: "closed" }>;

export type PropertyFormValues = {
  name: string;
  // null until a kind is chosen when adding.
  kind: PropertyKind | null;
  address: string;
};

// "address" is never found by the form (any text is fine); it is where an API error on it is
// shown.
export type PropertyFormErrors = Partial<
  Record<"name" | "kind" | "address", string>
>;

export type PropertyFormResult =
  | { ok: true; input: PropertyInput }
  | { ok: false; errors: PropertyFormErrors };

export type UnitFormValues = {
  label: string;
};

export type UnitFormErrors = Partial<Record<"label", string>>;

export type UnitFormResult =
  { ok: true; input: UnitInput } | { ok: false; errors: UnitFormErrors };

export type UnitOccupancyView =
  // The unit's tenant, shown as plain text.
  | { kind: "tenant"; name: string }
  // Occupied, but the API did not say by whom (the viewer may not read that tenant).
  | { kind: "occupied" }
  | { kind: "free" };
