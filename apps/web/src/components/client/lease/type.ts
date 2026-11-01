import type { z } from "zod";

import type {
  LeaseChargesInput,
  LeaseInput,
  LeaseTerms,
  TermMonths,
  Unit,
} from "@/lib/api/generated/model";

import type { leaseFormFieldsSchema, leaseTermsFieldsSchema } from "./utils";

// When the lease starts, against today in Jakarta: a lease entered late, one that starts today,
// or a booking.
export type StartTiming = "past" | "today" | "future";

// "Mulai tagih dari": bill every period since the start date, or from a later period start.
export type BillingChoice = "start" | "pick";

// One choice of the anniversary select.
export type DateOption = {
  // YYYY-MM-DD, the value sent as billedFrom.
  value: string;
  // "15 Okt 2026".
  label: string;
};

// The terms of a lease as the form holds them: what "Sewakan unit" and "Ubah sewa" share.
export type LeaseTermsValues = {
  // YYYY-MM-DD from the date input, "" until chosen.
  startDate: string;
  billing: BillingChoice;
  // The chosen anniversary; only sent with billing "pick" while "Mulai tagih dari" shows.
  billedFrom: string;
  // The deposit field's text: digits and dots, empty for none.
  deposit: string;
  // A member id, or NO_COLLECTOR for none.
  collector: string;
};

export type LeaseFormValues = LeaseTermsValues & {
  tenantId: string;
  // "" until chosen.
  unitId: string;
  // "Masa sewa": a priced term of the chosen unit; null when no unit is chosen or none is priced.
  termMonths: TermMonths | null;
};

// "Sewakan unit" and "Ubah sewa" as react-hook-form holds them, under the API's field names.
export type LeaseFormFields = z.input<typeof leaseFormFieldsSchema>;
export type LeaseTermsFields = z.input<typeof leaseTermsFieldsSchema>;

// What "Sewakan unit"'s schema needs besides the fields: the tenant, today in Jakarta, and the
// property and free unit the form shows (the only property is chosen already; a unit taken since
// is not shown).
export type LeaseFormContext = {
  tenantId: string;
  today: string;
  propertyId: string;
  unit: Pick<Unit, "id" | "terms"> | undefined;
};

// The hint under "Masa sewa", and whether it needs the member's attention.
export type TermHint = { text: string; tone: "muted" | "warning" };

// The fields a check or the API can name. "propertyId" is the form's own: the API never sees it.
export type LeaseFormField =
  | "propertyId"
  | "unitId"
  | "termMonths"
  | "startDate"
  | "billedFrom"
  | "depositAmount"
  | "collectorMemberId";

export type LeaseFormErrors = Partial<Record<LeaseFormField, string>>;

export type LeaseFormResult =
  { ok: true; input: LeaseInput } | { ok: false; errors: LeaseFormErrors };

export type LeaseTermsResult =
  { ok: true; input: LeaseTerms } | { ok: false; errors: LeaseFormErrors };

// What sits under "Mulai sewa" in "Ubah sewa": the billing choice for a lease entered late, the
// booking note for a start after today, or nothing.
export type EditStartBox = "billing" | "booking" | "none";

// One choice of "Masa sewa": a term the unit offers, disabled while it has no price.
export type TermOption = {
  // The term's months as text ("12"); a select item holds a string.
  value: string;
  // "Per tahun · Rp15.000.000" or "Per 3 bulan · belum ada harga".
  label: string;
  disabled: boolean;
};

// How the lease forms name one billing period of a term, in a sentence.
export type TermPeriodNames = {
  // "bulan", "periode 3 bulan", "periode 6 bulan", "tahun".
  noun: string;
  // Once the length is clear from the sentence: "bulan", "periode", "periode", "tahun".
  short: string;
  // "bulan-bulan", "periode-periode", "periode-periode", "tahun-tahun".
  plural: string;
};

// One choice of the collector select.
export type CollectorOption = {
  value: string;
  label: string;
};

// A date field's check: the date to send, or the message under the field.
export type DateCheck =
  { ok: true; date: string } | { ok: false; error: string };

// "Rencana keluar"'s suggestion: the day before the next period starts, and that next start.
export type PlannedEndSuggestion = {
  date: string;
  nextStart: string;
};

// "Biaya tambahan" (boards Sewa-Biaya.dc.html and Sewa-Biaya-Phone.dc.html).

// What the form holds for one charge type: whether it is checked, and the amount field's text
// (digits and dots, empty for none). An unchecked row keeps its text but is not sent.
export type ChargeEntry = {
  checked: boolean;
  amount: string;
};

// The entries by charge type id; a type without one is unchecked and empty.
export type ChargeEntries = Readonly<Record<string, ChargeEntry>>;

// One row of the form: a non-metered charge type and its entry.
export type ChargeRow = ChargeEntry & {
  chargeTypeId: string;
  name: string;
};

// The message under each bad row, by charge type id.
export type ChargeErrors = Partial<Record<string, string>>;

// What the check sends: the request, and the charge type id of the row sent at each index, so
// an API error on charges[i] goes back under its row. Or the rows' errors, and the first bad
// row in display order.
export type LeaseChargesResult =
  | { ok: true; input: LeaseChargesInput; sent: readonly string[] }
  | { ok: false; errors: ChargeErrors; first: string };

// "Nama biaya" in "Jenis biaya baru": the trimmed name to send, or the message under the field.
export type ChargeTypeNameCheck =
  { ok: true; name: string } | { ok: false; error: string };
