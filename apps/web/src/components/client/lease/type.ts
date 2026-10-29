import type {
  LeaseChargesInput,
  LeaseInput,
  LeaseTerms,
  TermMonths,
} from "@/lib/api/generated/model";

// When the lease starts, against today in Jakarta: a lease entered late, one that starts today,
// or a booking.
export type StartTiming = "past" | "today" | "future";

// "Mulai tagih dari": bill every period since the start date, or from a later anniversary.
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
  // The chosen unit's shortest term; null when no unit is chosen or it offers none.
  termMonths: TermMonths | null;
};

// The fields a check or the API can name. "propertyId" is the form's own: the API never sees it.
export type LeaseFormField =
  | "propertyId"
  | "unitId"
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
