import { z } from "zod";

import {
  NO_DATE_MESSAGE,
  shortestTerm,
  termPeriodWords,
} from "@/components/client/lease/utils";
import { ApiError } from "@/lib/api/errors";
import type { SortOptions } from "@/components/client/list/type";
import type {
  ListPropertiesSort,
  ListUnitsSort,
  Property,
  ScheduledPrice,
  TermMonths,
  Unit,
  UnitPrice,
  UnitTermInput,
} from "@/lib/api/generated/model";
import { PropertyKind } from "@/lib/api/generated/model";
import { formatDate, formatTimestampDate } from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { everywhere, type AppAbility } from "@/lib/capabilities";

import type {
  PropertyPermissions,
  UnitFormErrors,
  UnitFormResult,
  UnitFormValues,
  UnitCustomTermRow,
  UnitCustomTermView,
  LastTermPrices,
  UnitOccupancyView,
  UnitPriceFormErrors,
  UnitPriceFormResult,
  UnitPriceFormValues,
  UnitPriceHistoryRowView,
  UnitPriceTermView,
  UnitRowAction,
  UnitTermRow,
  UnitTermRowView,
} from "./type";

// The property and unit forms look like every other panel form.
export {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";

// The note under the title when the member can look but not change.
export const READ_ONLY_NOTE =
  "Anda hanya bisa melihat. Untuk mengubah, minta admin.";

export const ONLY_ASSIGNED_NOTE =
  "Anda hanya melihat properti yang ditugaskan kepada Anda.";

export const LOADING_TEXT = "Memuat…";

// The sort choices of the properties list and of a property's units; the API's default is first.
export const PROPERTY_SORT_OPTIONS: SortOptions<ListPropertiesSort> = [
  { value: "name", label: "Nama A–Z" },
  { value: "-name", label: "Nama Z–A" },
];

export const UNIT_SORT_OPTIONS: SortOptions<ListUnitsSort> = [
  { value: "label", label: "Nama unit A–Z" },
  { value: "-label", label: "Nama unit Z–A" },
];

// getProperty answers not_found for a property that does not exist (or an id that is not one)
// and forbidden for one outside the member's properties. The page shows both as "Properti tidak
// ditemukan", so it never tells a limited member that a property they may not see exists.
export function isPropertyMissing(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    (error.code === "not_found" || error.code === "forbidden")
  );
}

// Which property and unit controls show, from the member's ability (getOrgMe's capabilities),
// never from role names. "conditional" shows the control and lets the API decide per property;
// adding a property needs `property:manage` for the whole organization. Hiding is visibility
// only; the API checks every request.
export function propertyPermissions(ability: AppAbility): PropertyPermissions {
  return {
    canAddProperty: ability.can("manage", everywhere("property")),
    canEditProperty: ability.can("manage", "property"),
    canManageUnits: ability.can("manage", "unit"),
    seesOnlyAssigned:
      ability.can("read", "property") &&
      !ability.can("read", everywhere("property")),
  };
}

// The button at the end of each unit's row. Reading units is enough to see their prices (the API's
// listUnitPrices needs unit:read).
export function unitRowAction(
  permissions: PropertyPermissions,
  ability: AppAbility,
): UnitRowAction {
  if (permissions.canManageUnits) return "edit";
  return ability.can("read", "unit") ? "prices" : "none";
}

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

// A unit's usual rent: the price of its shortest term that has one, "Rp1.200.000" for a month,
// "Rp3.500.000 per 3 bulan" or "Rp15.000.000 per tahun" for a longer term, or "Belum diisi" when
// no term has a price in force.
export function unitRentText(unit: Pick<Unit, "terms">): string {
  const priced = unit.terms.filter((term) => term.price !== undefined);
  const months = shortestTerm(priced);
  const price = priced.find((term) => term.months === months)?.price;
  if (months === null || price === undefined) return NO_RENT_TEXT;
  const amount = formatRupiah(price);
  return months === 1 ? amount : `${amount} per ${termPeriodWords(months)}`;
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

// "Tambah properti" and "Ubah properti": the name and the address are sent trimmed, and an empty
// address clears it when editing.
export const propertyFormSchema = z.object({
  name: z.string().trim().min(1, EMPTY_PROPERTY_NAME_MESSAGE),
  kind: z.enum(PropertyKind, { error: NO_KIND_MESSAGE }),
  address: z.string().trim(),
});

// The property form's fields an API error can name; an error on any other field goes to the alert.
export const PROPERTY_FORM_API_FIELDS = ["name", "kind", "address"] as const;

export const NO_TERM_MESSAGE = "Pilih minimal satu masa sewa.";
export const EMPTY_CUSTOM_MONTHS_MESSAGE = "Isi jumlah bulan.";
export const STANDARD_CUSTOM_MONTHS_MESSAGE =
  "Untuk 1, 3, 6 atau 12 bulan, centang pilihannya di atas.";

export function checkUnitForm(
  values: UnitFormValues,
  unit: Pick<Unit, "terms"> | undefined,
): UnitFormResult {
  const label = values.label.trim();
  const errors: UnitFormErrors = {};
  if (label === "") errors.label = EMPTY_UNIT_LABEL_MESSAGE;
  const checked = values.terms.filter((row) => row.checked);
  if (checked.length === 0 && !values.custom.checked) {
    errors.terms = NO_TERM_MESSAGE;
  }
  // A price of Rp0 is refused under its own field, as "Ubah harga" does.
  for (const row of checked) {
    if (unitTermRowView(unit, row.months).kind === "priced") continue;
    if (isZeroPrice(row.priceText)) {
      errors.prices = { ...errors.prices, [row.months]: ZERO_PRICE_MESSAGE };
    }
  }
  Object.assign(errors, checkCustomTerm(values.custom, unit));

  if (Object.keys(errors).length > 0) return { ok: false, errors };
  const custom = customTermInput(values.custom, unit);
  return {
    ok: true,
    input: {
      label,
      terms: [
        ...checked.map((row) => termInput(row, unit)),
        ...(custom === null ? [] : [custom]),
      ],
    },
  };
}

// A checked term as the API takes it: its price only when the admin typed one in a field the row
// shows. A term with a price in force shows no field, so it sends none and keeps its price (the
// API records a row only for a price given).
function termInput(
  row: UnitTermRow,
  unit: Pick<Unit, "terms"> | undefined,
): UnitTermInput {
  if (unitTermRowView(unit, row.months).kind === "priced") {
    return { months: row.months };
  }
  const price = row.priceText === "" ? null : parseRupiahInput(row.priceText);
  return price === null
    ? { months: row.months }
    : { months: row.months, price };
}

function isZeroPrice(priceText: string): boolean {
  return priceText !== "" && parseRupiahInput(priceText) === 0;
}

// The friendly checks of the custom row. The API judges the month count (2 to 60, not a standard
// term) and its message shows under "Masa sewa dan harga"; the browser only catches an empty
// "Jumlah bulan" and a standard term typed there.
function checkCustomTerm(
  custom: UnitCustomTermRow,
  unit: Pick<Unit, "terms"> | undefined,
): Pick<UnitFormErrors, "customMonths" | "customPrice"> {
  const errors: Pick<UnitFormErrors, "customMonths" | "customPrice"> = {};
  if (!custom.checked) return errors;
  const view = customTermView(custom, unit);
  if (view.kind === "lainnya") {
    if (custom.monthsText === "") {
      errors.customMonths = EMPTY_CUSTOM_MONTHS_MESSAGE;
    } else if (UNIT_TERM_CHOICES.includes(Number(custom.monthsText))) {
      errors.customMonths = STANDARD_CUSTOM_MONTHS_MESSAGE;
    }
  }
  const showsPriceField = view.kind === "lainnya" || view.term.kind === "input";
  if (showsPriceField && isZeroPrice(custom.priceText)) {
    errors.customPrice = ZERO_PRICE_MESSAGE;
  }
  return errors;
}

// The custom row as the API takes it, or null when it is not ticked. Like a standard row, its
// price goes only when typed in a field the row shows: a saved custom term with a price in force
// sends none and keeps its price.
function customTermInput(
  custom: UnitCustomTermRow,
  unit: Pick<Unit, "terms"> | undefined,
): UnitTermInput | null {
  if (!custom.checked) return null;
  const view = customTermView(custom, unit);
  const months = view.kind === "saved" ? view.months : customMonths(custom);
  if (months === null) return null;
  if (view.kind === "saved" && view.term.kind === "priced") return { months };
  const price =
    custom.priceText === "" ? null : parseRupiahInput(custom.priceText);
  return price === null ? { months } : { months, price };
}

// The month count typed in "Jumlah bulan", or null for none.
export function customMonths(custom: UnitCustomTermRow): TermMonths | null {
  return custom.monthsText === "" ? null : Number(custom.monthsText);
}

// "Jumlah bulan" keeps only the digits of what was typed or pasted.
export function monthsDigits(text: string): string {
  return text.replace(/\D/g, "");
}

// The custom term the saved unit offers (any month count other than 1, 3, 6 and 12), or null.
export function savedCustomTerm(
  unit: Pick<Unit, "terms"> | undefined,
): TermMonths | null {
  const term = unit?.terms.find((t) => !UNIT_TERM_CHOICES.includes(t.months));
  return term?.months ?? null;
}

// The custom row "Masa sewa dan harga" starts with: the saved unit's custom term, ticked, or
// "Lainnya", unticked, when it has none (always when adding a unit).
export function initialCustomTermRow(
  unit: Pick<Unit, "terms"> | undefined,
): UnitCustomTermRow {
  const saved = savedCustomTerm(unit);
  return {
    checked: saved !== null,
    showsSaved: saved !== null,
    monthsText: saved === null ? "" : String(saved),
    priceText: "",
  };
}

// Ticking or unticking the custom row. Ticking a saved custom term again after unticking it
// turns the row into "Lainnya" with its month count typed in, so the admin can choose another.
export function toggleCustomTerm(
  custom: UnitCustomTermRow,
  checked: boolean,
): UnitCustomTermRow {
  if (checked && !custom.checked && custom.showsSaved) {
    return { ...custom, checked, showsSaved: false };
  }
  return { ...custom, checked };
}

// What the custom row shows: the saved custom term like a standard row, or "Lainnya" with its
// fields. The price field of "Lainnya" says what an empty field means for the typed month count:
// a count the history has a price for gets that price back.
export function customTermView(
  custom: UnitCustomTermRow,
  unit: Pick<Unit, "terms"> | undefined,
  lastPrices: LastTermPrices = {},
): UnitCustomTermView {
  const saved = savedCustomTerm(unit);
  if (custom.showsSaved && saved !== null) {
    return {
      kind: "saved",
      months: saved,
      term: unitTermRowView(unit, saved, lastPrices),
    };
  }
  const months = customMonths(custom);
  return {
    kind: "lainnya",
    price: {
      kind: "input",
      nextPrice: undefined,
      lastPrice: months === null ? undefined : lastPrices[months],
    },
  };
}

export const CUSTOM_TERM_LABEL = "Lainnya";
export const CUSTOM_MONTHS_LABEL = "Jumlah bulan";
// The range decision 33 allows; a hint only, the API judges it.
export const CUSTOM_MONTHS_HINT = "2 sampai 60 bulan, selain 3, 6 dan 12.";

// The price field of "Lainnya": "Harga per 4 bulan" once a month count is typed, "Harga per
// periode" before.
export function customTermPriceLabel(custom: UnitCustomTermRow): string {
  const months = customMonths(custom);
  return months === null || months < 1
    ? "Harga per periode"
    : termPriceLabel(months);
}

// The terms a unit may offer, shortest first: the rows of "Masa sewa dan harga".
export const UNIT_TERM_CHOICES: readonly TermMonths[] = [1, 3, 6, 12];

// The term a new unit offers to begin with, as the API's default: a year for a ruko, a month for
// a kos, kontrakan or other unit.
export function defaultUnitTerm(kind: PropertyKind): TermMonths {
  return kind === "ruko" ? 12 : 1;
}

// The rows "Masa sewa dan harga" starts with: the unit's offered terms when changing one, the
// property kind's default when adding.
export function initialTermRows(
  unit: Pick<Unit, "terms"> | undefined,
  kind: PropertyKind,
): UnitTermRow[] {
  return UNIT_TERM_CHOICES.map((months) => ({
    months,
    checked:
      unit === undefined
        ? months === defaultUnitTerm(kind)
        : unit.terms.some((term) => term.months === months),
    priceText: "",
  }));
}

// What a checked term shows: its price in force, read-only, when the saved unit offers it with
// one; else a field for a price (adding, a newly checked term, or an offered term with no price).
// A term the unit no longer offers has no price on the unit, but its last price in the history
// applies again when the term is offered with no price (lastPrices, from lastTermPrices).
export function unitTermRowView(
  unit: Pick<Unit, "terms"> | undefined,
  months: TermMonths,
  lastPrices: LastTermPrices = {},
): UnitTermRowView {
  const term = unit?.terms.find((t) => t.months === months);
  if (term?.price !== undefined) {
    return { kind: "priced", price: term.price, nextPrice: term.nextPrice };
  }
  return {
    kind: "input",
    nextPrice: term?.nextPrice,
    lastPrice: lastPrices[months],
  };
}

// Each term's latest price on or before today, from the history as the API orders it (latest
// effective date first, and on one date the latest recorded first), so its first such row wins.
export function lastTermPrices(
  history: readonly UnitPrice[],
  today: string,
): LastTermPrices {
  const prices: LastTermPrices = {};
  for (const price of history) {
    if (price.effectiveFrom > today || prices[price.months] !== undefined) {
      continue;
    }
    prices[price.months] = price.amount;
  }
  return prices;
}

// The hint under a term's price field: what an empty field means.
export function termPriceHint(
  view: Extract<UnitTermRowView, { kind: "input" }>,
): string {
  if (view.lastPrice === undefined) return TERM_PRICE_HINT;
  return `Kalau dikosongkan, harga terakhir ${formatRupiah(view.lastPrice)} berlaku lagi.`;
}

// The admin unchecked a term the unit offers: leases on it go on as before.
// A saved custom term is dropped when its row is unticked, or re-ticked as "Lainnya" with another
// month count.
export function dropsOfferedTerm(
  unit: Pick<Unit, "terms"> | undefined,
  rows: readonly UnitTermRow[],
  custom: UnitCustomTermRow,
): boolean {
  if (unit === undefined) return false;
  const saved = savedCustomTerm(unit);
  if (
    saved !== null &&
    !(custom.checked && (custom.showsSaved || customMonths(custom) === saved))
  ) {
    return true;
  }
  return rows.some(
    (row) =>
      !row.checked && unit.terms.some((term) => term.months === row.months),
  );
}

export const DROPPED_TERM_NOTE =
  "Sewa yang sudah memakai masa sewa ini tetap berjalan seperti biasa.";
export const TERMS_LEGEND = "Masa sewa dan harga";
export const TERMS_HINT =
  "Centang masa sewa yang ditawarkan. Harga untuk satu periode penuh, bukan harga bulanan dikali.";
export const TERM_PRICE_HINT =
  "Boleh dikosongkan. Tanpa harga, masa sewa ini belum bisa dipakai untuk sewa baru.";

// "Per bulan", "Per 3 bulan", "Per tahun".
export function termLabel(months: TermMonths): string {
  return `Per ${termPeriodWords(months)}`;
}

// "Harga per tahun".
export function termPriceLabel(months: TermMonths): string {
  return `Harga per ${termPeriodWords(months)}`;
}

// "Ubah harga per tahun", the name of a term's "Ubah harga" button.
export function changePriceLabel(months: TermMonths): string {
  return `Ubah harga per ${termPeriodWords(months)}`;
}

// "Mulai 1 Jan 2027: Rp1.600.000".
export function nextPriceText(next: ScheduledPrice): string {
  return `Mulai ${formatDate(next.effectiveFrom)}: ${formatRupiah(next.amount)}`;
}

export const UNIT_PRICES_TITLE = "Harga unit";
export const PRICES_BUTTON_TEXT = "Harga";

// "Lihat harga Kamar 3", the name of a unit's "Harga" button.
export function pricesButtonLabel(unitLabel: string): string {
  return `Lihat harga ${unitLabel}`;
}
export const NO_PRICE_TEXT = "Belum ada harga";

// "Harga unit": the unit's offered terms, shortest first, each with its price in force and the one
// still to come.
export function unitPriceTermViews(
  unit: Pick<Unit, "terms">,
): UnitPriceTermView[] {
  return [...unit.terms]
    .sort((a, b) => a.months - b.months)
    .map((term) => ({
      months: term.months,
      label: termLabel(term.months),
      price:
        term.price === undefined ? NO_PRICE_TEXT : formatRupiah(term.price),
      next: term.nextPrice === undefined ? null : nextPriceText(term.nextPrice),
    }));
}

// "Riwayat harga": the API's rows in its order (latest effective date first). The first row of a
// term on or before today is its price in force; a row after today is still to come.
export function priceHistoryRows(
  history: readonly UnitPrice[],
  today: string,
): UnitPriceHistoryRowView[] {
  const inForce = new Set<TermMonths>();
  return history.map((price) => {
    let chip: UnitPriceHistoryRowView["chip"] = null;
    if (price.effectiveFrom > today) {
      chip = FUTURE_PRICE_CHIP;
    } else if (!inForce.has(price.months)) {
      inForce.add(price.months);
      chip = CURRENT_PRICE_CHIP;
    }
    const recorded =
      price.recordedByName === undefined
        ? MIGRATED_PRICE_TEXT
        : `dicatat ${price.recordedByName}, ${formatTimestampDate(price.recordedAt)}`;
    return {
      id: price.id,
      title: `${termLabel(price.months)} · ${formatRupiah(price.amount)}`,
      detail: `Mulai ${formatDate(price.effectiveFrom)} · ${recorded}`,
      chip,
    };
  });
}

export const FUTURE_PRICE_CHIP = "Akan berlaku";
export const CURRENT_PRICE_CHIP = "Berlaku sekarang";
export const MIGRATED_PRICE_TEXT = "dari data sebelumnya";
export const PRICE_HISTORY_TITLE = "Riwayat harga";
export const NO_PRICE_HISTORY_TEXT = "Belum ada harga yang dicatat.";
export const PRICE_HISTORY_ERROR_TEXT = "Riwayat harga tidak bisa dimuat.";

// "Ubah harga": what the form checks before it sends a new price.

export const EMPTY_PRICE_MESSAGE = "Isi harga baru.";
export const ZERO_PRICE_MESSAGE = "Harga harus lebih dari Rp0.";
export const PRICE_BEFORE_TODAY_MESSAGE =
  "Tanggal tidak boleh sebelum hari ini.";
export const PRICE_DATE_HINT =
  "Tagihan memakai harga yang berlaku pada hari tagihan dibuat. Harga lama tetap tercatat di riwayat.";
export const PRICE_AMOUNT_HINT = "Untuk satu periode penuh.";

// The API fields "Ubah harga" shows under its own fields.
export const UNIT_PRICE_API_FIELDS: readonly string[] = [
  "months",
  "amount",
  "effectiveFrom",
];

export function checkUnitPriceForm(
  values: UnitPriceFormValues,
  today: string,
): UnitPriceFormResult {
  const errors: UnitPriceFormErrors = {};
  const amount =
    values.amountText === "" ? null : parseRupiahInput(values.amountText);
  if (amount === null) errors.amount = EMPTY_PRICE_MESSAGE;
  else if (amount <= 0) errors.amount = ZERO_PRICE_MESSAGE;
  if (values.effectiveFrom === "") errors.effectiveFrom = NO_DATE_MESSAGE;
  else if (values.effectiveFrom < today) {
    errors.effectiveFrom = PRICE_BEFORE_TODAY_MESSAGE;
  }
  if (amount === null || Object.keys(errors).length > 0) {
    return { ok: false, errors };
  }
  return {
    ok: true,
    input: {
      months: values.months,
      amount,
      effectiveFrom: values.effectiveFrom,
    },
  };
}

// "Harga sekarang: Rp1.500.000", or "Harga sekarang: belum ada".
export function currentPriceText(
  unit: Pick<Unit, "terms">,
  months: TermMonths,
): string {
  const price = unit.terms.find((term) => term.months === months)?.price;
  return `Harga sekarang: ${price === undefined ? "belum ada" : formatRupiah(price)}`;
}

// The unit's offered terms as "Masa sewa" choices, shortest first.
export function priceTermOptions(
  unit: Pick<Unit, "terms">,
): { value: string; label: string }[] {
  return [...unit.terms]
    .sort((a, b) => a.months - b.months)
    .map((term) => ({
      value: String(term.months),
      label: termLabel(term.months),
    }));
}

// The line at the top of the unit's form once a price is saved:
// "Harga per tahun dicatat, berlaku mulai 1 Jan 2027."
export function priceSavedNotice(
  price: Pick<UnitPrice, "months" | "effectiveFrom">,
): string {
  return `Harga per ${termPeriodWords(price.months)} dicatat, berlaku mulai ${formatDate(price.effectiveFrom)}.`;
}
