import type { LeaseInput } from "@/lib/api/generated/model";

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

export type LeaseFormValues = {
  tenantId: string;
  // "" until chosen.
  unitId: string;
  // YYYY-MM-DD from the date input, "" until chosen.
  startDate: string;
  billing: BillingChoice;
  // The chosen anniversary; only sent with billing "pick" on a start before today.
  billedFrom: string;
  // The amount fields' text: digits and dots, empty for none.
  rent: string;
  deposit: string;
  // A member id, or NO_COLLECTOR for none.
  collector: string;
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
