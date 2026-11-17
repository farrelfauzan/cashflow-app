import type { FormEvent } from "react";
import type { UseFormReturn } from "react-hook-form";
import type { z } from "zod";

import type { ReceiveCashTenant } from "@/components/client/field-cash/type";
import type { FieldPromiseTenant } from "@/components/client/field-promise/type";
import type { FieldChip } from "@/components/client/field-today/type";
import type { FieldVisit, VisitOutcomeKind } from "@/lib/api/generated/model";

import type { visitOutcomeSchema } from "./utils";

// One tenant on Kunjungan hari ini (FieldVisit), ready to show.
export type FieldVisitRowView = {
  tenantId: string;
  tenantName: string;
  // "Kamar 11 · Kos Melati", several units joined by ", "; empty without one.
  place: string;
  // "Sisa Rp2.900.000 · telat Rp2.200.000", "Sisa Rp1.050.000" or "Lunas".
  owed: string;
  // Open rows: why the visit is open ("Telat 9 hari", "Janji lewat 2×", "WA gagal terkirim").
  reasons: readonly FieldChip[];
  // Open rows: "Tadi tidak bertemu, 10.20" after a not_home outcome today; null otherwise.
  notHomeLine: string | null;
  // Done rows: what was recorded ("Sudah bayar", "Janji bayar", "Nomor baru"); null for none.
  doneChip: FieldChip | null;
  // Done rows: "Tunai dicatat 09.15" or "Dicatat 09.15"; null without an outcome.
  doneLine: string | null;
  // "Arahkan" opens this in a maps app; null when no unit has an address.
  directionsUrl: string | null;
  done: boolean;
  // The API's row, for the sheet it opens.
  visit: FieldVisit;
};

// The open rows (the API's order: the oldest visit first) and those recorded today.
export type FieldVisitSections = {
  open: readonly FieldVisitRowView[];
  done: readonly FieldVisitRowView[];
};

// One of the four answers of "Hasil kunjungan".
export type VisitOutcomeOption = {
  value: VisitOutcomeKind;
  label: string;
  sub: string;
};

// "Rp1.000.000 · 10.42 · foto kwitansi tersimpan": one cash payment taken today that an outcome
// may name.
export type VisitCashChoiceView = {
  paymentId: string;
  label: string;
};

// "Janji Jum, 20 Nov · semua sisa" and "Dicatat 10.31": today's promise an outcome may name.
export type VisitPromiseSummaryView = {
  promiseVersionId: string;
  terms: string;
  recorded: string;
};

// What the form holds, as react-hook-form keeps it. The cash and the promise an outcome names are
// not typed: they come from the list (unlinkedCashToday, unlinkedPromiseToday).
export type VisitOutcomeFormValues = z.input<typeof visitOutcomeSchema>;

// What the schema hands on: a chosen outcome, the number trimmed, the note trimmed.
export type VisitOutcomeFormOutput = z.output<typeof visitOutcomeSchema>;

// The records an outcome points at, from the list at the moment Simpan is pressed.
export type VisitOutcomeLinks = {
  paymentId: string | null;
  promiseVersionId: string | null;
};

// What a failed recordVisitOutcome shows.
export type VisitErrorView =
  // No answer, or a server failure: the outcome may be recorded. Everything stays as sent and
  // Simpan sends the same request again, which answers 200 if it was.
  | { kind: "unknown"; message: string }
  // Under "Nomor WA baru" or "Catatan".
  | { kind: "field"; field: "newPhone" | "note"; message: string }
  // The list behind the sheet is out of date (no open visit any more, the cash or the promise
  // cannot be named): the API's message at the top, and the list is reloaded.
  | { kind: "stale"; message: string }
  // At the top of the sheet (403, a tenant gone, anything else).
  | { kind: "alert"; message: string };

// One opening of a sheet for one tenant: its requestId, kept while the app does not know whether
// its save reached the API ("unsure"), so the same opening comes back instead of a fresh one.
export type TenantOpening<T> = {
  tenantId: string;
  requestId: string;
  unsure: boolean;
  item: T;
};

// useTenantOpenings: the openings of one kind of sheet on a screen, and which one shows.
export type TenantOpenings<T> = {
  openings: readonly TenantOpening<T>[];
  // The tenant whose sheet shows while `open`.
  activeTenantId: string | null;
  open: boolean;
  start: (item: T) => void;
  setOpen: (open: boolean) => void;
  markUnsure: (tenantId: string, unsure: boolean) => void;
};

// The tenant a nested "Terima tunai" is for, and the place line under its title.
export type VisitCashSheetItem = {
  tenantId: string;
  tenant: ReceiveCashTenant;
  placeLabel: string;
};

// The tenant a nested "Catat janji bayar" is for; `key` mounts the sheet afresh for each opening.
export type VisitPromiseSheetItem = {
  tenantId: string;
  tenant: FieldPromiseTenant;
  placeLabel: string;
  key: number;
};

// useRecordVisit: one opening of "Catat hasil kunjungan".
export type RecordVisit = {
  form: UseFormReturn<VisitOutcomeFormValues, unknown, VisitOutcomeFormOutput>;
  // The outcome chosen; "" before one is.
  outcome: VisitOutcomeFormValues["outcome"];
  alert: string | undefined;
  cashChoices: readonly VisitCashChoiceView[];
  // The cash "Sudah bayar hari ini" names: the one picked, else the newest.
  paymentId: string | null;
  choosePayment: (paymentId: string) => void;
  promise: VisitPromiseSummaryView | null;
  canSave: boolean;
  saving: boolean;
  // A save got no answer, so the outcome may be recorded: nothing can change until the API
  // answers, and Simpan sends the same request again.
  locked: boolean;
  dirty: boolean;
  submit: (event: FormEvent<HTMLFormElement>) => void;
  // The sheet closed: reload the lists if an outcome was (or may have been) recorded.
  finish: () => void;
};
