import { retakePayment } from "@/components/client/receipt-retake/utils";
import type {
  FieldCashToday,
  FieldTodayItem,
  FieldUnit,
  ReceiptPhotoStatus,
} from "@/lib/api/generated/model";
import { formatRupiah } from "@/lib/format/rupiah";

import type { FieldCashLineView, FieldChip, FieldTodayRowView } from "./type";

// "Tagihan hari ini" (board HP-Tagihan, column 1): the tenants the karyawan collects from today,
// as listFieldToday orders them. Every amount shown is a field the API sent, only formatted:
// nothing here adds or subtracts rupiah.

export const TITLE = "Tagihan hari ini";
export const LIST_LABEL = "Daftar tagihan hari ini";
export const EMPTY_TEXT = "Tidak ada tagihan untuk hari ini.";
export const FORBIDDEN_TEXT = "Anda tidak punya akses ke tagihan hari ini.";

export const CASH_CARD_LABEL = "Tunai di tangan, belum disetor";
export const DEPOSIT_LINK_TEXT = "Setor ke admin";

// One page of the list, the API's maximum: a karyawan's day is far shorter.
export const FIELD_TODAY_PAGE_SIZE = 100;

// "Kamar 11 · Kos Melati"; a tenant renting several units the karyawan collects shows each.
export function placeText(units: readonly FieldUnit[]): string {
  return units
    .map((unit) => `${unit.unitLabel} · ${unit.propertyName}`)
    .join(", ");
}

// What the tenant still owes, with the overdue part when there is one: "Sisa Rp1.050.000 · telat
// Rp1.050.000"; "Lunas" once nothing is owed (a tenant who paid in cash today).
export function owedText(
  item: Pick<FieldTodayItem, "outstanding" | "overdue">,
): string {
  if (item.outstanding <= 0) return "Lunas";
  const rest = `Sisa ${formatRupiah(item.outstanding)}`;
  return item.overdue > 0
    ? `${rest} · telat ${formatRupiah(item.overdue)}`
    : rest;
}

// Why the tenant is on the list, from the oldest invoice they still owe on (FieldDueReason).
export function reasonChip(
  item: Pick<FieldTodayItem, "reason" | "daysOverdue" | "daysUntilDue">,
): FieldChip {
  switch (item.reason) {
    case "overdue":
      return {
        label:
          item.daysOverdue === undefined
            ? "Telat"
            : `Telat ${item.daysOverdue} hari`,
        tone: "warning",
      };
    case "due_today":
      return { label: "Jatuh tempo hari ini", tone: "neutral" };
    case "due_in_days":
      return {
        label:
          item.daysUntilDue === 2 ? "Jatuh tempo lusa" : "Jatuh tempo besok",
        tone: "neutral",
      };
    case "paid_today":
      return { label: "Tunai diterima", tone: "active" };
  }
}

// The photo of today's cash. A refused one is retaken with "Ambil ulang" on its payment's line
// under the chip.
const PHOTO_CHIPS: Record<ReceiptPhotoStatus, FieldChip> = {
  clean: { label: "Foto tersimpan", tone: "active" },
  pending: { label: "Foto sedang diperiksa", tone: "none" },
  missing: { label: "Foto belum ada", tone: "warning" },
  rejected: { label: "Foto ditolak", tone: "warning" },
  infected: { label: "Foto ditolak", tone: "warning" },
};

// With several payments today the chip shows the one that needs attention most.
const PHOTO_PRIORITY: readonly ReceiptPhotoStatus[] = [
  "rejected",
  "infected",
  "missing",
  "pending",
  "clean",
];

export function photoChip(statuses: readonly ReceiptPhotoStatus[]): FieldChip {
  const status =
    PHOTO_PRIORITY.find((candidate) => statuses.includes(candidate)) ?? "clean";
  return PHOTO_CHIPS[status];
}

// "Tunai hari ini Rp950.000" with its photo; null when no cash was taken from the tenant today.
export function cashLine(cash: FieldCashToday): FieldCashLineView | null {
  if (cash.count === 0 || cash.payments.length === 0) return null;
  return {
    text: `Tunai hari ini ${formatRupiah(cash.amount)}`,
    photo: photoChip(cash.payments.map((payment) => payment.photo.status)),
  };
}

export function fieldTodayRow(item: FieldTodayItem): FieldTodayRowView {
  const place = placeText(item.units);
  return {
    tenantId: item.tenantId,
    tenantName: item.tenantName,
    place,
    owed: owedText(item),
    reason: reasonChip(item),
    cash: cashLine(item.cashToday),
    cashPayments: item.cashToday.payments.map((payment) =>
      retakePayment(payment, item, place),
    ),
    item,
    canCollect: item.outstanding > 0,
  };
}

// Under the list when the API holds more rows than one page shows: "dan 3 penyewa lainnya".
export function moreRowsText(total: number, shown: number): string | null {
  const more = total - shown;
  return more > 0 ? `dan ${more} penyewa lainnya` : null;
}
