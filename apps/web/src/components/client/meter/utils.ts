import type { QueryKey } from "@tanstack/react-query";
import { z } from "zod";

import { invoiceCorrectionTitle } from "@/components/client/billing/utils";
import { ApiError } from "@/lib/api/errors";
import type {
  Lease,
  MeteredCharge,
  MeteredChargeInput,
  NextInvoice,
} from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";
import {
  formatDayMonth,
  formatMonthYear,
  formatTimestampDate,
} from "@/lib/format/date";
import { formatRupiah, parseRupiahInput } from "@/lib/format/rupiah";
import { rupiahFieldText } from "@/lib/format/rupiah-input";

import type {
  LockedMeterRow,
  LockedMeterView,
  MeterFormOutput,
  MeterSelection,
  MonthOption,
  RecordedMeterRowView,
} from "./type";

// "Catat meteran" (W14). The admin types the rupiah amount; the meter numbers are only a record.
// Nothing here adds, subtracts or derives rupiah: every amount shown is a field the API sent.

export const METER_PANEL_TITLE = "Catat meteran";

// MeteredChargeInput.amount's maximum.
export const METER_AMOUNT_MAX = 1_000_000_000_000;
// Fifteen digits always fit in a safe integer.
export const METER_NUMBER_MAX_DIGITS = 15;
// "Bulan pemakaian" offers at most this many months.
export const MAX_MONTH_OPTIONS = 12;
// "Sudah dicatat" shows the first page of the lease's amounts.
export const RECORDED_PAGE_SIZE = 20;

export const EMPTY_MONTH_MESSAGE = "Pilih bulan pemakaian.";
export const EMPTY_TYPE_MESSAGE = "Pilih jenis biaya.";
export const EMPTY_AMOUNT_MESSAGE = "Isi jumlah tagihan.";
export const DIGITS_ONLY_MESSAGE = "Hanya angka.";
export const AMOUNT_ZERO_MESSAGE = "Jumlah harus lebih dari Rp0.";
export const AMOUNT_TOO_LARGE_MESSAGE = `Jumlah paling banyak ${formatRupiah(METER_AMOUNT_MAX)}.`;
export const METER_END_BELOW_START_MESSAGE =
  "Angka akhir tidak boleh lebih kecil dari angka awal.";

export const MONTH_HINT = "Biasanya bulan lalu.";
export const METER_NUMBERS_LABEL = "Angka meteran (opsional)";
export const METER_NUMBERS_HINT =
  "Hanya untuk catatan. Yang ditagihkan adalah jumlah di bawah.";
export const AMOUNT_LABEL = "Jumlah tagihan";
export const AMOUNT_HINT = "Hanya angka.";
export const NO_NEXT_INVOICE_TEXT =
  "Belum ada tagihan berikutnya untuk sewa ini.";

export const RECORDED_TITLE = "Sudah dicatat";
export const RECORDED_EMPTY_TEXT = "Belum ada meteran yang dicatat.";
export const RECORDED_LOADING_TEXT = "Memuat…";
export const UNBILLED_CHIP = "Belum ditagihkan";
export const LOCKED_CHIP = "Sudah ditagihkan, terkunci";
export const EDIT_TEXT = "Ubah";

export const SAVE_NEW_LABEL = "Simpan meteran";
export const SAVE_CHANGE_LABEL = "Simpan perubahan";

// The fields an API error can name; any other goes to the form's alert.
export const METER_API_FIELDS = [
  "usageMonth",
  "chargeTypeId",
  "amount",
  "meterStart",
  "meterEnd",
] as const;

const USAGE_MONTH = /^(\d{4})-(0[1-9]|1[0-2])$/;

// "2026-11-04" (or "2026-11") gives "2026-11".
export function monthOf(date: string): string {
  return date.slice(0, 7);
}

// The month before "2026-01" is "2025-12".
export function previousMonth(month: string): string {
  const match = USAGE_MONTH.exec(month);
  if (match === null) throw new RangeError(`not a month: ${month}`);
  const year = Number(match[1]);
  const index = Number(match[2]);
  if (index === 1) return `${year - 1}-12`;
  return `${year}-${String(index - 1).padStart(2, "0")}`;
}

// "2026-10" gives "Oktober 2026".
export function monthLabel(month: string): string {
  return formatMonthYear(`${month}-01`);
}

// "Bulan pemakaian": newest first, from last month back to the lease's start month, at most 12.
// A lease that started this month offers only this month; one that starts later (a booking)
// offers none. The API refuses months before the start month and after this month.
export function usageMonthOptions(
  startDate: string,
  today: string,
): MonthOption[] {
  const start = monthOf(startDate);
  const current = monthOf(today);
  if (start > current) return [];
  if (start === current)
    return [{ value: current, label: monthLabel(current) }];
  const options: MonthOption[] = [];
  let month = previousMonth(current);
  while (month >= start && options.length < MAX_MONTH_OPTIONS) {
    options.push({ value: month, label: monthLabel(month) });
    month = previousMonth(month);
  }
  return options;
}

// "Catat meteran" shows on an active lease that has started and expects at least one metered
// type, to a member with charge:record ("conditional" shows it; the API decides per lease).
// Never by role name.
export function canRecordMeter(ability: AppAbility): boolean {
  return ability.can("record", "charge");
}

export function meterAvailable(
  lease: Pick<Lease, "status" | "startDate" | "meteredChargeTypes">,
  today: string,
): boolean {
  return (
    lease.status === "active" &&
    lease.startDate <= today &&
    lease.meteredChargeTypes.length > 0
  );
}

// The form's starting values: the newest month offered, the lease's first metered type, nothing
// typed.
export function meterFormDefaults(
  months: readonly MonthOption[],
  lease: Pick<Lease, "meteredChargeTypes">,
) {
  return {
    usageMonth: months[0]?.value ?? "",
    chargeTypeId: lease.meteredChargeTypes[0]?.chargeTypeId ?? "",
    meterStart: "",
    meterEnd: "",
    amount: "",
  };
}

// What "Awal" and "Akhir" keep of a typed or pasted value: digits only, at most 15.
export function meterDigits(input: string): string {
  return input.replace(/\D/g, "").slice(0, METER_NUMBER_MAX_DIGITS);
}

// The amount and meter fields for a recorded amount (prefilled for a change), or empty.
export function meterFieldValues(
  charge: Pick<MeteredCharge, "amount" | "meterStart" | "meterEnd"> | undefined,
): { amount: string; meterStart: string; meterEnd: string } {
  if (charge === undefined) return { amount: "", meterStart: "", meterEnd: "" };
  return {
    amount: rupiahFieldText(String(charge.amount)),
    meterStart:
      charge.meterStart === undefined ? "" : String(charge.meterStart),
    meterEnd: charge.meterEnd === undefined ? "" : String(charge.meterEnd),
  };
}

const meterNumber = z.string().transform((text, ctx) => {
  const trimmed = text.trim();
  if (trimmed === "") return undefined;
  if (!/^\d+$/.test(trimmed)) {
    ctx.addIssue({ code: "custom", message: DIGITS_ONLY_MESSAGE });
    return z.NEVER;
  }
  return Number(trimmed);
});

// The friendly early checks of "Catat meteran". The Go API checks everything again and its
// message wins.
export const meterFormSchema = z
  .object({
    usageMonth: z.string().regex(USAGE_MONTH, EMPTY_MONTH_MESSAGE),
    chargeTypeId: z.string().min(1, EMPTY_TYPE_MESSAGE),
    meterStart: meterNumber,
    meterEnd: meterNumber,
    amount: z.string().transform((text, ctx) => {
      if (text.trim() === "") {
        ctx.addIssue({ code: "custom", message: EMPTY_AMOUNT_MESSAGE });
        return z.NEVER;
      }
      const amount = parseRupiahInput(text);
      if (amount === null) {
        ctx.addIssue({ code: "custom", message: DIGITS_ONLY_MESSAGE });
        return z.NEVER;
      }
      if (amount <= 0) {
        ctx.addIssue({ code: "custom", message: AMOUNT_ZERO_MESSAGE });
        return z.NEVER;
      }
      if (amount > METER_AMOUNT_MAX) {
        ctx.addIssue({ code: "custom", message: AMOUNT_TOO_LARGE_MESSAGE });
        return z.NEVER;
      }
      return amount;
    }),
  })
  .superRefine((values, ctx) => {
    if (
      values.meterStart !== undefined &&
      values.meterEnd !== undefined &&
      values.meterEnd < values.meterStart
    ) {
      ctx.addIssue({
        code: "custom",
        path: ["meterEnd"],
        message: METER_END_BELOW_START_MESSAGE,
      });
    }
  });

// The request: the month and type go in the path (usageMonth as YYYY-MM), the integer amount
// and the meter numbers that were typed in the body.
export function meterRequest(fields: MeterFormOutput): {
  chargeTypeId: string;
  usageMonth: string;
  data: MeteredChargeInput;
} {
  const data: MeteredChargeInput = { amount: fields.amount };
  if (fields.meterStart !== undefined) data.meterStart = fields.meterStart;
  if (fields.meterEnd !== undefined) data.meterEnd = fields.meterEnd;
  return {
    chargeTypeId: fields.chargeTypeId,
    usageMonth: fields.usageMonth,
    data,
  };
}

// One key per month and type.
export function selectionKey(selection: MeterSelection): string {
  return `${selection.usageMonth}|${selection.chargeTypeId}`;
}

// The recorded amount of the month and type, from a list the API sent.
export function findCharge(
  charges: readonly MeteredCharge[] | undefined,
  selection: MeterSelection,
): MeteredCharge | undefined {
  return charges?.find(
    (charge) =>
      charge.usageMonth === selection.usageMonth &&
      charge.chargeTypeId === selection.chargeTypeId,
  );
}

// "Listrik Oktober 2026".
export function chargeName(
  charge: Pick<MeteredCharge, "chargeTypeName" | "usageMonth">,
): string {
  return `${charge.chargeTypeName} ${monthLabel(charge.usageMonth)}`;
}

// The warning when the month and type already have an amount not yet billed.
export function existingAmountText(charge: MeteredCharge): string {
  return `${chargeName(charge)} sudah dicatat ${formatRupiah(charge.amount)} oleh ${charge.recordedBy.name}. Menyimpan akan mengganti jumlahnya.`;
}

// The form's note when the month and type are billed and the locked view was closed.
export function lockedSelectionText(charge: MeteredCharge): string {
  return `${chargeName(charge)} sudah ditagihkan dan terkunci.`;
}

// After a save: "Listrik Oktober 2026 Rp123.000 tersimpan. Masuk Tagihan November 2026."
export function savedText(
  charge: MeteredCharge,
  nextInvoice: NextInvoice | undefined,
): string {
  const saved = `${chargeName(charge)} ${formatRupiah(charge.amount)} tersimpan.`;
  if (nextInvoice === undefined) return saved;
  return `${saved} Masuk Tagihan ${formatMonthYear(nextInvoice.period)}.`;
}

// Which invoice an amount recorded now goes on.
export function nextInvoiceText(nextInvoice: NextInvoice | undefined): string {
  if (nextInvoice === undefined) return NO_NEXT_INVOICE_TEXT;
  return `Masuk ke tagihan berikutnya yang terbit setelah ini dicatat: Tagihan ${formatMonthYear(nextInvoice.period)}, terbit ${formatDayMonth(nextInvoice.issuedOn)}.`;
}

export function recordedRows(
  charges: readonly MeteredCharge[],
): RecordedMeterRowView[] {
  return charges.map((charge) => ({
    id: charge.id,
    selection: {
      usageMonth: charge.usageMonth,
      chargeTypeId: charge.chargeTypeId,
    },
    text: `${charge.chargeTypeName} · ${monthLabel(charge.usageMonth)} · ${formatRupiah(charge.amount)}`,
    name: chargeName(charge),
    locked: charge.billed !== undefined,
  }));
}

export function lockedText(period: string): string {
  return `Jumlah ini sudah masuk Tagihan ${formatMonthYear(period)} dan tidak bisa diubah lagi. Kalau salah, buat koreksi pada tagihan itu dengan alasannya.`;
}

// The locked view of a billed amount; null for one no invoice bills.
export function lockedView(charge: MeteredCharge): LockedMeterView | null {
  if (charge.billed === undefined) return null;
  const rows: LockedMeterRow[] = [
    { label: "Jumlah", value: formatRupiah(charge.amount) },
  ];
  if (charge.meterStart !== undefined) {
    rows.push({ label: "Angka awal", value: String(charge.meterStart) });
  }
  if (charge.meterEnd !== undefined) {
    rows.push({ label: "Angka akhir", value: String(charge.meterEnd) });
  }
  rows.push({
    label: "Dicatat",
    value: `${charge.recordedBy.name} · ${formatTimestampDate(charge.updatedAt)}`,
  });
  return {
    heading: `${charge.chargeTypeName} · ${monthLabel(charge.usageMonth)}`,
    rows,
    text: lockedText(charge.billed.period),
    invoiceId: charge.billed.invoiceId,
    correctLabel: invoiceCorrectionTitle(charge.billed),
  };
}

// The API refused because an invoice bills the month's amount meanwhile.
export function isBilledError(error: unknown): boolean {
  return error instanceof ApiError && error.code === "metered_charge_billed";
}

// Every query of the organization's missing meter readings (Penagihan's flag). Keys are
// ["org", orgId, url, params] (src/lib/api/query-key.ts).
export function isMissingMetersQueryKey(
  queryKey: QueryKey,
  orgId: string,
): boolean {
  const [scope, keyOrg, url] = queryKey;
  if (scope !== "org" || keyOrg !== orgId || typeof url !== "string") {
    return false;
  }
  const base = `/orgs/${orgId}/metered-charges`;
  return (
    url === base || url.startsWith(`${base}/`) || url.startsWith(`${base}?`)
  );
}
