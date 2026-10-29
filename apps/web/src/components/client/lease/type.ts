import type { LeaseInput, LeaseTerms } from "@/lib/api/generated/model";

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
  // The amount fields' text: digits and dots, empty for none.
  rent: string;
  deposit: string;
  // A member id, or NO_COLLECTOR for none.
  collector: string;
};

export type LeaseFormValues = LeaseTermsValues & {
  tenantId: string;
  // "" until chosen.
  unitId: string;
};

// The fields a check or the API can name. "propertyId" is the form's own: the API never sees it.
export type LeaseFormField =
  | "propertyId"
  | "unitId"
  | "startDate"
  | "billedFrom"
  | "rentAmount"
  | "depositAmount"
  | "collectorMemberId";

export type LeaseFormErrors = Partial<Record<LeaseFormField, string>>;

export type LeaseFormResult =
  { ok: true; input: LeaseInput } | { ok: false; errors: LeaseFormErrors };

export type LeaseTermsResult =
  { ok: true; input: LeaseTerms } | { ok: false; errors: LeaseFormErrors };

// The rent and deposit typed in a lease form, as integers: rent null when it is not a whole
// amount of at least 1, deposit undefined when the field is empty.
export type LeaseAmounts = {
  rent: number | null;
  deposit: number | undefined;
};

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
