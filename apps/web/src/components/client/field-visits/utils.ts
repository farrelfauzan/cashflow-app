import { z } from "zod";

import { photoLine } from "@/components/client/field-cash/utils";
import type { ReceiveCashTenant } from "@/components/client/field-cash/type";
import type { FieldChip } from "@/components/client/field-today/type";
import { owedText, placeText } from "@/components/client/field-today/utils";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type {
  FieldCashPayment,
  FieldPromiseVersion,
  FieldUnit,
  FieldVisit,
  VisitOutcome,
  VisitOutcomeInput,
  VisitReason,
} from "@/lib/api/generated/model";
import { VisitOutcomeKind } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";
import {
  formatTimestampTime,
  formatTimestampWeekdayDayMonth,
  formatWeekdayDayMonth,
} from "@/lib/format/date";
import { formatRupiah } from "@/lib/format/rupiah";

import type {
  FieldVisitRowView,
  FieldVisitSections,
  TenantOpening,
  VisitCashChoiceView,
  VisitErrorView,
  VisitOutcomeLinks,
  VisitOutcomeOption,
  VisitPromiseSummaryView,
} from "./type";

// "Kunjungan hari ini" and "Catat hasil kunjungan" (board HP-Kunjungan, T-101). The list is
// listFieldVisits as the API orders it; an outcome points at what the field app already recorded
// (Terima tunai, Catat janji bayar), so no amount is typed here. Every amount shown is a field the
// API sent, only formatted.

export const TITLE = "Kunjungan hari ini";
export const DATE_NOTE = "disusun otomatis setiap pagi";
export const LIST_LABEL = "Daftar kunjungan hari ini";
export const DONE_TITLE = "Sudah dicatat hari ini";
export const DONE_LIST_LABEL = "Kunjungan yang sudah dicatat hari ini";
export const EMPTY_TEXT = "Tidak ada kunjungan hari ini.";
// Every visit of the day has its outcome; the done rows follow.
export const ALL_DONE_TEXT = "Semua kunjungan hari ini sudah dicatat.";
export const FORBIDDEN_TEXT = "Anda tidak punya akses ke kunjungan hari ini.";

export const DIRECTIONS_TEXT = "Arahkan";
export const RECORD_TEXT = "Catat hasil";

// One page of the list, the API's maximum: a karyawan's day is far shorter.
export const FIELD_VISITS_PAGE_SIZE = 100;

export const SHEET_TITLE = "Catat hasil kunjungan";
export const OUTCOME_LEGEND = "Hasil kunjungan";
export const CASH_LIST_LABEL = "Tunai yang dicatat hari ini";
export const NO_CASH_TEXT =
  "Belum ada tunai dari penyewa ini hari ini. Terima dulu dengan foto kwitansinya.";
export const RECEIVE_CASH_TEXT = "Terima tunai";
export const RECORD_PROMISE_TEXT = "Catat janji bayar";
export const CHANGE_PROMISE_TEXT = "Ubah janji";
export const PHONE_LABEL = "Nomor WA baru";
export const PHONE_PLACEHOLDER = "0812-3456-7890";
export const PHONE_HINT =
  "Admin yang menggantinya setelah dicek. Pesan berikutnya ke nomor ini.";
export const NOTE_LABEL = "Catatan";
export const NOTE_OPTIONAL = "(boleh kosong)";
export const NOTE_MAX_LENGTH = 500;
export const SAVE_TEXT = "Simpan hasil";
export const SAVING_TEXT = "Menyimpan…";

export const CHOOSE_OUTCOME_MESSAGE = "Pilih hasil kunjungan.";
export const EMPTY_PHONE_MESSAGE = "Isi nomor WA baru.";
export const NOTE_TOO_LONG_MESSAGE = "Catatan paling banyak 500 karakter.";
export const UNKNOWN_SAVE_TEXT =
  "Sinyal hilang, belum pasti tersimpan. Tekan Simpan hasil lagi; hasil tidak akan tercatat dua kali.";

export const SAVED_TOAST_TITLE = "Hasil kunjungan tersimpan";

const MAPS_SEARCH_URL = "https://www.google.com/maps/search/?api=1&query=";

// The four answers, in the board's order.
export const OUTCOME_OPTIONS: readonly VisitOutcomeOption[] = [
  {
    value: VisitOutcomeKind.paid,
    label: "Sudah bayar hari ini",
    sub: "Tunai yang sudah dicatat dengan foto kwitansi",
  },
  {
    value: VisitOutcomeKind.promise,
    label: "Janji bayar baru",
    sub: "Tanggal yang disepakati dengan penyewa",
  },
  {
    value: VisitOutcomeKind.not_home,
    label: "Tidak bertemu",
    sub: "Tetap di daftar besok",
  },
  {
    value: VisitOutcomeKind.number_changed,
    label: "Nomor WA berubah",
    sub: "Admin menggantinya setelah dicek",
  },
];

const OUTCOME_VALUES = OUTCOME_OPTIONS.map((option) => option.value);

function isOutcomeKind(value: string): value is VisitOutcomeKind {
  return (OUTCOME_VALUES as readonly string[]).includes(value);
}

// The fields of the form an API error can name; any other goes to the alert.
export const VISIT_API_FIELDS = ["newPhone", "note"] as const;
const STALE_CODES: ReadonlySet<string> = new Set([
  "visit_not_open",
  "visit_payment_invalid",
  "visit_promise_invalid",
]);
const REQUEST_ID_REUSED_CODE = "request_id_reused";

// "Catat hasil" shows to a member who may record visit outcomes on some tenants or all
// ("conditional": the API decides per tenant). Never by role name.
export function canRecordVisit(ability: AppAbility): boolean {
  return ability.can("record", "visit");
}

// "Senin, 16 Nov · disusun otomatis setiap pagi".
export function headerDateLine(now: Date): string {
  return `${formatTimestampWeekdayDayMonth(now)} · ${DATE_NOTE}`;
}

// One chip per open visit, oldest first: "Telat 9 hari", "Janji lewat 2×", "WA gagal terkirim".
// A visit whose debt is no longer overdue (daysOverdue 0) stays open until an outcome closes it,
// and says only "Telat".
export function reasonChip(reason: VisitReason): FieldChip {
  switch (reason.code) {
    case "overdue":
      return {
        label:
          reason.daysOverdue === undefined || reason.daysOverdue <= 0
            ? "Telat"
            : `Telat ${reason.daysOverdue} hari`,
        tone: "warning",
      };
    case "broken_promises":
      return {
        label:
          reason.brokenPromises === undefined || reason.brokenPromises <= 0
            ? "Janji lewat"
            : `Janji lewat ${reason.brokenPromises}×`,
        tone: "warning",
      };
    case "undeliverable":
      return { label: "WA gagal terkirim", tone: "warning" };
  }
}

// "Tadi tidak bertemu, 10.20" under an open row the karyawan already found nobody at today.
export function notHomeLine(outcome: VisitOutcome | undefined): string | null {
  if (outcome?.outcome !== VisitOutcomeKind.not_home) return null;
  return `Tadi tidak bertemu, ${formatTimestampTime(outcome.recordedAt)}`;
}

// The chip of a row recorded today; not_home records nothing to show.
export function doneChip(outcome: VisitOutcome | undefined): FieldChip | null {
  switch (outcome?.outcome) {
    case VisitOutcomeKind.paid:
      return { label: "Sudah bayar", tone: "active" };
    case VisitOutcomeKind.promise:
      return { label: "Janji bayar", tone: "neutral" };
    case VisitOutcomeKind.number_changed:
      return { label: "Nomor baru", tone: "neutral" };
    default:
      return null;
  }
}

// The grey line of a row recorded today (board HP-Kunjungan). A paid outcome carries the amount
// its cash counts now, so the line names it with the time: "Rp950.000 tunai · 09.15". Without an
// amount it says only when it was recorded, as every other outcome does: "Tunai dicatat 09.15",
// "Dicatat 09.15".
export function doneLine(outcome: VisitOutcome | undefined): string | null {
  if (outcome === undefined) return null;
  const time = formatTimestampTime(outcome.recordedAt);
  if (outcome.outcome !== VisitOutcomeKind.paid) return `Dicatat ${time}`;
  return outcome.amount === undefined
    ? `Tunai dicatat ${time}`
    : `${formatRupiah(outcome.amount)} tunai · ${time}`;
}

// "Arahkan" searches the first address of the tenant's units in Google Maps (a link: nothing is
// loaded from Google by this page); null when no unit has one.
export function directionsUrl(units: readonly FieldUnit[]): string | null {
  const address = units
    .map((unit) => unit.propertyAddress.trim())
    .find((text) => text !== "");
  return address === undefined
    ? null
    : `${MAPS_SEARCH_URL}${encodeURIComponent(address)}`;
}

export function fieldVisitRow(visit: FieldVisit): FieldVisitRowView {
  const done = visit.status === "done";
  return {
    tenantId: visit.tenantId,
    tenantName: visit.tenantName,
    place: placeText(visit.units),
    owed: owedText(visit),
    reasons: done ? [] : visit.reasons.map(reasonChip),
    notHomeLine: done ? null : notHomeLine(visit.lastOutcome),
    doneChip: done ? doneChip(visit.lastOutcome) : null,
    doneLine: done ? doneLine(visit.lastOutcome) : null,
    directionsUrl: directionsUrl(visit.units),
    done,
    visit,
  };
}

// The open rows first, then "Sudah dicatat hari ini", each in the API's order.
export function fieldVisitSections(
  visits: readonly FieldVisit[],
): FieldVisitSections {
  const rows = visits.map(fieldVisitRow);
  return {
    open: rows.filter((row) => !row.done),
    done: rows.filter((row) => row.done),
  };
}

// What "Terima tunai" shows of the tenant (its owed box), from their visit: the overdue line when
// part of the debt is past due, else the oldest due date alone. Nothing is computed: the days
// come from the open overdue visit, the amounts from the row.
export function receiveCashTenant(visit: FieldVisit): ReceiveCashTenant {
  const overdue = visit.reasons.find((reason) => reason.code === "overdue");
  const days =
    overdue?.daysOverdue !== undefined && overdue.daysOverdue > 0
      ? overdue.daysOverdue
      : undefined;
  return {
    tenantId: visit.tenantId,
    tenantName: visit.tenantName,
    reason: visit.overdue > 0 ? "overdue" : "paid_today",
    daysOverdue: visit.overdue > 0 ? days : undefined,
    daysUntilDue: undefined,
    oldestUnpaidDueDate: visit.oldestUnpaidDueDate,
    outstanding: visit.outstanding,
  };
}

// "foto kwitansi tersimpan", in the middle of a line.
function photoText(payment: FieldCashPayment): string {
  const title = photoLine(payment.photo.status).title;
  return `${title.charAt(0).toLowerCase()}${title.slice(1)}`;
}

// "Rp1.000.000 · 10.42 · foto kwitansi tersimpan", newest first so the preselected one leads.
export function cashChoices(
  payments: readonly FieldCashPayment[],
): VisitCashChoiceView[] {
  return [...payments].reverse().map((payment) => ({
    paymentId: payment.paymentId,
    label: `${formatRupiah(payment.amount)} · ${formatTimestampTime(payment.recordedAt)} · ${photoText(payment)}`,
  }));
}

// The cash "Sudah bayar hari ini" names: the one picked while it can still be named, else the
// newest (the API lists them oldest first). Null when there is none.
export function selectedPaymentId(
  payments: readonly FieldCashPayment[],
  chosen: string | null,
): string | null {
  if (chosen !== null && payments.some((p) => p.paymentId === chosen)) {
    return chosen;
  }
  return payments.at(-1)?.paymentId ?? null;
}

// "Janji Jum, 20 Nov · semua sisa" (or "· Rp500.000") and "Dicatat 10.31".
export function promiseSummary(
  version: FieldPromiseVersion | null,
): VisitPromiseSummaryView | null {
  if (version === null) return null;
  const amount =
    version.promisedAmount === null
      ? "semua sisa"
      : formatRupiah(version.promisedAmount);
  return {
    promiseVersionId: version.promiseVersionId,
    terms: `Janji ${formatWeekdayDayMonth(version.promisedDate)} · ${amount}`,
    recorded: `Dicatat ${formatTimestampTime(version.createdAt)}`,
  };
}

// The answer the sheet opens on: what the karyawan already recorded today for the tenant (cash
// first, as on the board), else none.
export function defaultOutcome(visit: FieldVisit): VisitOutcomeKind | "" {
  if (visit.unlinkedCashToday.length > 0) return VisitOutcomeKind.paid;
  if (visit.unlinkedPromiseToday !== null) return VisitOutcomeKind.promise;
  return "";
}

// "Simpan hasil" works once an answer is chosen and, for paid and promise, the record it names
// exists; not while it saves. The number is checked when it is pressed, so a missing one says why.
export function canSaveOutcome(
  outcome: string,
  links: VisitOutcomeLinks,
  saving: boolean,
): boolean {
  if (saving) return false;
  switch (outcome) {
    case VisitOutcomeKind.paid:
      return links.paymentId !== null;
    case VisitOutcomeKind.promise:
      return links.promiseVersionId !== null;
    case VisitOutcomeKind.not_home:
    case VisitOutcomeKind.number_changed:
      return true;
    default:
      return false;
  }
}

// The friendly early checks; the Go API checks everything again and its message wins.
export const visitOutcomeSchema = z
  .object({
    outcome: z.string(),
    newPhone: z.string(),
    note: z.string(),
  })
  .transform((values, ctx) => {
    const note = values.note.trim();
    if (note.length > NOTE_MAX_LENGTH) {
      ctx.addIssue({
        code: "custom",
        path: ["note"],
        message: NOTE_TOO_LONG_MESSAGE,
      });
    }
    const newPhone = values.newPhone.trim();
    if (values.outcome === VisitOutcomeKind.number_changed && newPhone === "") {
      ctx.addIssue({
        code: "custom",
        path: ["newPhone"],
        message: EMPTY_PHONE_MESSAGE,
      });
    }
    if (!isOutcomeKind(values.outcome)) {
      ctx.addIssue({
        code: "custom",
        path: ["outcome"],
        message: CHOOSE_OUTCOME_MESSAGE,
      });
      return z.NEVER;
    }
    return { outcome: values.outcome, newPhone, note };
  });

export const VISIT_FORM_DEFAULTS = { outcome: "", newPhone: "", note: "" };

// recordVisitOutcome's body: the answer with only the field it takes (the cash, the promise or
// the number), the note when there is one, and the opening's requestId. Null when the record a
// paid or promise answer names is missing.
export function visitOutcomeBody(
  fields: { outcome: VisitOutcomeKind; newPhone: string; note: string },
  links: VisitOutcomeLinks,
  requestId: string,
): VisitOutcomeInput | null {
  const body: VisitOutcomeInput = { requestId, outcome: fields.outcome };
  switch (fields.outcome) {
    case VisitOutcomeKind.paid:
      if (links.paymentId === null) return null;
      body.paymentId = links.paymentId;
      break;
    case VisitOutcomeKind.promise:
      if (links.promiseVersionId === null) return null;
      body.promiseVersionId = links.promiseVersionId;
      break;
    case VisitOutcomeKind.number_changed:
      body.newPhone = fields.newPhone;
      break;
    case VisitOutcomeKind.not_home:
      break;
  }
  if (fields.note !== "") body.note = fields.note;
  return body;
}

// What a failed recordVisitOutcome shows. No answer or a server failure may have recorded it, so
// the same request is sent again (the API answers 200 if it did). An error about the number or
// the note goes under it; one saying the list is out of date reloads it.
export function visitErrorView(error: unknown): VisitErrorView {
  if (!(error instanceof ApiError)) {
    return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
  }
  if (
    error.code === NETWORK_ERROR_CODE ||
    error.status === 0 ||
    error.status >= 500
  ) {
    return { kind: "unknown", message: UNKNOWN_SAVE_TEXT };
  }
  if (STALE_CODES.has(error.code)) {
    return { kind: "stale", message: error.message };
  }
  if (error.code === REQUEST_ID_REUSED_CODE) {
    return { kind: "alert", message: GENERIC_ERROR_MESSAGE };
  }
  if (error.field === "newPhone" || error.field === "note") {
    return { kind: "field", field: error.field, message: error.message };
  }
  return { kind: "alert", message: error.message };
}

// The openings kept when `item`'s tenant opens a sheet: every other tenant's opening still unsure
// whether it saved (so pressing its button again brings it back as it was), and this tenant's own
// unsure opening, or else a new one with `requestId`. Openings that are sure are dropped.
export function nextOpenings<T extends { tenantId: string }>(
  openings: readonly TenantOpening<T>[],
  item: T,
  requestId: string,
): TenantOpening<T>[] {
  const others = openings.filter(
    (opening) => opening.unsure && opening.tenantId !== item.tenantId,
  );
  const own = openings.find(
    (opening) => opening.tenantId === item.tenantId && opening.unsure,
  );
  return [
    ...others,
    own === undefined
      ? { tenantId: item.tenantId, requestId, unsure: false, item }
      : { ...own, item },
  ];
}

// Marks a tenant's opening unsure or sure; the same array when nothing changes, so a sheet that
// reports on every render causes no new one.
export function markOpening<T>(
  openings: readonly TenantOpening<T>[],
  tenantId: string,
  unsure: boolean,
): readonly TenantOpening<T>[] {
  const index = openings.findIndex((opening) => opening.tenantId === tenantId);
  const current = openings[index];
  if (current === undefined || current.unsure === unsure) return openings;
  return openings.map((opening, at) =>
    at === index ? { ...opening, unsure } : opening,
  );
}
