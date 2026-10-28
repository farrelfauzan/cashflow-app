import type {
  Capability,
  Property,
  PropertyKind,
  Unit,
} from "@/lib/api/generated/model";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { canSee } from "@/lib/organization/org-menu";

import type {
  PropertyFormErrors,
  PropertyFormResult,
  PropertyFormValues,
  PropertyPermissions,
  UnitFormErrors,
  UnitFormResult,
  UnitFormValues,
  UnitOccupancyView,
} from "./type";

// A text field of the property and unit forms: 52px tall, 17px text on iPad and 16px on phone.
export const INPUT_CLASS =
  "h-13 rounded-xl border-[1.5px] bg-card px-3.5 text-base aria-invalid:border-2 md:text-[17px]";

export const TEXTAREA_CLASS =
  "min-h-28 rounded-xl border-[1.5px] bg-card px-3.5 py-3 text-base aria-invalid:border-2 md:text-[17px]";

export const FIELD_LABEL_CLASS =
  "text-[15px] leading-normal font-semibold md:text-base";

export const FIELD_HINT_CLASS =
  "text-sm leading-[1.45] text-muted-foreground md:text-base";

// The note under the title when the member can look but not change.
export const READ_ONLY_NOTE =
  "Anda hanya bisa melihat. Untuk mengubah, minta admin.";

export const ONLY_ASSIGNED_NOTE =
  "Anda hanya melihat properti yang ditugaskan kepada Anda.";

export const LOADING_TEXT = "Memuat…";

// True only when the member may do `action` on every record ("allow"), not just on some
// ("conditional", for example a role limited to some properties). For controls the API accepts
// only for the whole organization, like adding a property.
export function canDoEverywhere(
  capabilities: readonly Capability[],
  action: string,
): boolean {
  return capabilities.find((c) => c.action === action)?.access === "allow";
}

// Which property and unit controls show, from the signed-in member's capabilities (getOrgMe),
// never from role names. "conditional" shows the control and lets the API decide per property.
// Hiding is visibility only; the API checks every request.

export function propertyPermissions(
  capabilities: readonly Capability[],
): PropertyPermissions {
  return {
    canAddProperty: canDoEverywhere(capabilities, "property:manage"),
    canEditProperty: canSee(capabilities, "property:manage"),
    canManageUnits: canSee(capabilities, "unit:manage"),
    seesOnlyAssigned:
      canSee(capabilities, "property:read") &&
      !canDoEverywhere(capabilities, "property:read"),
  };
}

export const NO_PROPERTY_PERMISSIONS: PropertyPermissions = {
  canAddProperty: false,
  canEditProperty: false,
  canManageUnits: false,
  seesOnlyAssigned: false,
};

// The list of properties changes nothing for this member.
export function propertiesReadOnly(permissions: PropertyPermissions): boolean {
  return !permissions.canEditProperty;
}

// One property's page has two kinds of change: the property itself and its units.
export function unitsReadOnly(permissions: PropertyPermissions): boolean {
  return !permissions.canEditProperty && !permissions.canManageUnits;
}

// The words the property screens show for what the API returns. Counts come from the API; this
// only puts them in a sentence.

export const PROPERTY_KIND_LABELS: Record<PropertyKind, string> = {
  kos: "Kos",
  kontrakan: "Kontrakan",
  ruko: "Ruko",
  other: "Lainnya",
};

// The order of the kind choices in the form.
export const PROPERTY_KINDS: readonly PropertyKind[] = [
  "kos",
  "kontrakan",
  "ruko",
  "other",
];

export function propertyKindLabel(kind: PropertyKind): string {
  return PROPERTY_KIND_LABELS[kind];
}

// "10 unit".
export function unitCountText(property: Pick<Property, "unitCount">): string {
  return `${property.unitCount} unit`;
}

// "8 terisi, 2 kosong". A property with no unit yet says so instead of "0 terisi, 0 kosong".
export function occupancyText(
  property: Pick<Property, "unitCount" | "occupiedUnitCount">,
): string {
  if (property.unitCount === 0) return "Belum ada unit";
  const free = property.unitCount - property.occupiedUnitCount;
  return `${property.occupiedUnitCount} terisi, ${free} kosong`;
}

// The line under a property's name on its page: "Jl. Melati No. 12, Sleman · Kos · 10 unit".
// A property with no address leaves the address out.
export function propertySummary(
  property: Pick<Property, "address" | "kind" | "unitCount">,
): string {
  return [
    property.address.trim(),
    propertyKindLabel(property.kind),
    unitCountText(property),
  ]
    .filter((part) => part !== "")
    .join(" · ");
}

export const NO_RENT_TEXT = "Belum diisi";

// A unit's usual rent: "Rp1.200.000", or "Belum diisi" when none is set.
export function defaultRentText(unit: Pick<Unit, "defaultRent">): string {
  return unit.defaultRent === undefined
    ? NO_RENT_TEXT
    : formatRupiah(unit.defaultRent);
}

export function unitOccupancyView(
  unit: Pick<Unit, "occupied" | "currentTenant">,
): UnitOccupancyView {
  if (!unit.occupied) return { kind: "free" };
  if (unit.currentTenant !== undefined) {
    return { kind: "tenant", name: unit.currentTenant.name };
  }
  return { kind: "occupied" };
}

export const OCCUPIED_LABEL = "Terisi";
export const FREE_LABEL = "Kosong";

// What the property and unit forms check before they send anything. The Go API validates
// everything again and its message always wins; these only give a friendly message early.

// The API's limits (PropertyInput and UnitInput).
export const PROPERTY_NAME_MAX_LENGTH = 100;
export const PROPERTY_ADDRESS_MAX_LENGTH = 300;
export const UNIT_LABEL_MAX_LENGTH = 50;

export const EMPTY_PROPERTY_NAME_MESSAGE = "Isi nama properti.";
export const NO_KIND_MESSAGE = "Pilih jenis properti.";
export const EMPTY_UNIT_LABEL_MESSAGE = "Isi nama unit.";
export const BAD_RENT_MESSAGE = "Isi sewa dengan angka, atau kosongkan.";

export function checkPropertyForm(
  values: PropertyFormValues,
): PropertyFormResult {
  const name = values.name.trim();
  const errors: PropertyFormErrors = {};
  if (name === "") errors.name = EMPTY_PROPERTY_NAME_MESSAGE;
  if (values.kind === null) errors.kind = NO_KIND_MESSAGE;
  if (values.kind === null || Object.keys(errors).length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    input: { name, kind: values.kind, address: values.address.trim() },
  };
}

export function checkUnitForm(values: UnitFormValues): UnitFormResult {
  const label = values.label.trim();
  const errors: UnitFormErrors = {};
  if (label === "") errors.label = EMPTY_UNIT_LABEL_MESSAGE;

  // An empty rent means none: the field is optional and the API clears it when it is left out.
  let defaultRent: number | undefined;
  if (values.rent.trim() !== "") {
    const rent = parseRupiahInput(values.rent);
    if (rent === null || rent < 1) errors.defaultRent = BAD_RENT_MESSAGE;
    else defaultRent = rent;
  }

  if (Object.keys(errors).length > 0) return { ok: false, errors };
  return {
    ok: true,
    input: defaultRent === undefined ? { label } : { label, defaultRent },
  };
}
