import type {
  PropertyInput,
  PropertyKind,
  ScheduledPrice,
  TermMonths,
  UnitInput,
  UnitPriceInput,
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

// One row of "Masa sewa dan harga": whether the unit offers the term, and the price typed for it
// (digits grouped with dots, "" for none).
export type UnitTermRow = {
  months: TermMonths;
  checked: boolean;
  priceText: string;
};

// The fifth row of "Masa sewa dan harga": the unit's one custom term (decision 33). While
// `showsSaved` is true the row is the saved unit's custom term ("Per 4 bulan") and behaves like a
// standard row; otherwise it is "Lainnya", with "Jumlah bulan" and a price. A saved custom term
// unticked and ticked again turns into "Lainnya" with its months typed in, so another count can be
// chosen.
export type UnitCustomTermRow = {
  checked: boolean;
  showsSaved: boolean;
  // "Jumlah bulan": digits only, "" for none.
  monthsText: string;
  // The price typed for it (digits grouped with dots, "" for none).
  priceText: string;
};

export type UnitFormValues = {
  label: string;
  // Always the four standard terms, shortest first.
  terms: readonly UnitTermRow[];
  custom: UnitCustomTermRow;
};

export type UnitFormErrors = Partial<
  Record<"label" | "terms" | "customMonths" | "customPrice", string>
> & {
  // Under a standard term's price field.
  prices?: Partial<Record<TermMonths, string>>;
};

// What a checked term shows under its checkbox: the price in force, read-only, with the next
// scheduled change and "Ubah harga"; or a field for a price. A term the unit no longer offers
// keeps its last price in the history, which applies again when the field stays empty.
export type UnitTermRowView =
  | { kind: "priced"; price: number; nextPrice: ScheduledPrice | undefined }
  | {
      kind: "input";
      nextPrice: ScheduledPrice | undefined;
      lastPrice: number | undefined;
    };

// What the custom row shows: the saved unit's custom term as a standard row would, or "Lainnya"
// with "Jumlah bulan" and a price field.
export type UnitCustomTermView =
  | { kind: "saved"; months: TermMonths; term: UnitTermRowView }
  | {
      kind: "lainnya";
      price: Extract<UnitTermRowView, { kind: "input" }>;
    };

// The latest price on or before today of each term in a unit's history.
export type LastTermPrices = Partial<Record<TermMonths, number>>;

// Which view the unit panel shows: the unit's form, or "Ubah harga" for one term.
export type UnitPanelView =
  { kind: "unit" } | { kind: "price"; months: TermMonths };

// One row of "Riwayat harga".
export type UnitPriceHistoryRowView = {
  id: string;
  // "Per tahun · Rp15.000.000".
  title: string;
  // "Mulai 1 Jan 2027 · dicatat Pak Hendra Gunawan, 30 Okt 2026".
  detail: string;
  chip: "Akan berlaku" | "Berlaku sekarang" | null;
};

export type UnitPriceFormValues = {
  months: TermMonths;
  amountText: string;
  // YYYY-MM-DD, "" for none.
  effectiveFrom: string;
};

export type UnitPriceFormErrors = Partial<
  Record<"months" | "amount" | "effectiveFrom", string>
>;

export type UnitPriceFormResult =
  | { ok: true; input: UnitPriceInput }
  | { ok: false; errors: UnitPriceFormErrors };

export type UnitFormResult =
  { ok: true; input: UnitInput } | { ok: false; errors: UnitFormErrors };

export type UnitOccupancyView =
  // The unit's tenant, shown as plain text.
  | { kind: "tenant"; name: string }
  // Occupied, but the API did not say by whom (the viewer may not read that tenant).
  | { kind: "occupied" }
  | { kind: "free" };
