import type { CorrectionRowView } from "@/components/client/billing/type";
import type {
  Dashboard,
  DashboardCorrection,
  TenantArrears,
} from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";
import {
  formatDate,
  formatMonth,
  formatMonthYear,
  formatTimestampDate,
} from "@/lib/format/date";
import { formatRupiah, formatSignedRupiah } from "@/lib/format/rupiah";

import type { ArrearsRowView, ArrearsTotalView, MonthMoneyView } from "./type";

// Beranda. Every amount shown is a field getDashboard sent, only formatted: nothing here adds or
// subtracts rupiah. The one ratio is how full the month's bar is drawn.

export const DASHBOARD_LOADING_TEXT = "Memuat ringkasan…";
export const DASHBOARD_REGION_LABEL = "Ringkasan uang";
export const NO_REPORT_TEXT = "Pilih menu untuk mulai bekerja.";

export const ARREARS_TOTAL_TITLE = "Tunggakan";
export const NO_ARREARS_TOTAL_NOTE = "Tidak ada tunggakan";

export const ARREARS_TITLE = "Belum bayar";
export const ARREARS_LIST_LABEL = "Penyewa yang belum bayar";
export const NO_ARREARS_TEXT = "Tidak ada penyewa yang terlambat bayar.";

export const CORRECTIONS_TITLE = "Koreksi terbaru";
export const CORRECTIONS_LIST_LABEL = "Daftar koreksi terbaru";
export const NO_CORRECTIONS_TEXT = "Belum ada koreksi nominal.";

// "Selamat pagi" until 11, "siang" until 15, "sore" until 18, then "malam" (Jakarta hours).
export function greeting(hour: number): string {
  if (hour >= 4 && hour < 11) return "Selamat pagi";
  if (hour >= 11 && hour < 15) return "Selamat siang";
  if (hour >= 15 && hour < 18) return "Selamat sore";
  return "Selamat malam";
}

// How full the month's bar is: the share of what is billed that came in, rounded down so the
// bar is full only when everything is paid. Null when nothing is billed.
export function collectedPercent(
  collected: number,
  billed: number,
): number | null {
  if (billed <= 0) return null;
  const percent = Math.floor((collected * 100) / billed);
  return Math.min(100, Math.max(0, percent));
}

// "Uang sewa bulan November", from the month the API answered for (YYYY-MM).
export function monthMoney(dashboard: Dashboard): MonthMoneyView {
  const title = `Uang sewa bulan ${formatMonth(`${dashboard.period}-01`)}`;
  const billed = formatRupiah(dashboard.billed);
  const summary: MonthMoneyView["summary"] =
    dashboard.invoiceCount === 0 || dashboard.billed === 0
      ? { kind: "none" }
      : dashboard.outstanding === 0
        ? { kind: "paid", billed }
        : {
            kind: "owing",
            billed,
            outstanding: formatRupiah(dashboard.outstanding),
            tenants: dashboard.tenantsOutstanding,
          };
  return {
    title,
    collected: formatRupiah(dashboard.collected),
    percent: collectedPercent(dashboard.collected, dashboard.billed),
    summary,
  };
}

// "Tunggakan": what every tenant owes after the due dates, of any month.
export function arrearsTotal(dashboard: Dashboard): ArrearsTotalView {
  if (dashboard.overdue <= 0) {
    return {
      value: formatRupiah(0),
      note: NO_ARREARS_TOTAL_NOTE,
      tone: "default",
    };
  }
  return {
    value: formatRupiah(dashboard.overdue),
    note: `${dashboard.tenantsInArrears} penyewa terlambat bayar, semua bulan`,
    tone: "warning",
  };
}

// "Kamar 11, Kos Melati"; a debt over several units names the first and counts the rest.
export function arrearsPlace(arrears: TenantArrears): string {
  const [first, ...rest] = arrears.units;
  if (first === undefined) return "";
  const place = `${first.unitLabel}, ${first.propertyName}`;
  return rest.length === 0 ? place : `${place} +${rest.length} unit lain`;
}

// The rows of "Belum bayar", in the API's order (the oldest debt first, then the most owed).
export function arrearsRows(dashboard: Dashboard): ArrearsRowView[] {
  return dashboard.arrears.map((arrears) => ({
    tenantId: arrears.tenantId,
    name: arrears.tenantName,
    place: arrearsPlace(arrears),
    amount: formatRupiah(arrears.overdue),
    age: `Terlambat ${arrears.ageDays} hari`,
  }));
}

// "· 9 orang" after "Belum bayar": every tenant in arrears, not only the ones listed.
export function arrearsCount(dashboard: Dashboard): string {
  return `${dashboard.tenantsInArrears} orang`;
}

// "dan 3 penyewa lain" under the list when the API listed only the first ten; null otherwise.
export function moreArrearsText(dashboard: Dashboard): string | null {
  const more = dashboard.tenantsInArrears - dashboard.arrears.length;
  return more > 0 ? `dan ${more} penyewa lain` : null;
}

// What a correction changed: "Tagihan November 2026 · Kamar 11, Kos Melati" or
// "Pembayaran 25 Nov 2026".
function correctionSubject(correction: DashboardCorrection): string {
  if (correction.period !== undefined) {
    const invoice = `Tagihan ${formatMonthYear(correction.period)}`;
    return correction.unit === undefined
      ? invoice
      : `${invoice} · ${correction.unit.unitLabel}, ${correction.unit.propertyName}`;
  }
  if (correction.receivedOn !== undefined) {
    return `Pembayaran ${formatDate(correction.receivedOn)}`;
  }
  return "Koreksi";
}

// The rows of "Koreksi terbaru", in the API's order (newest first), in the look of the tenant
// screen's "Koreksi nominal".
export function correctionRows(dashboard: Dashboard): CorrectionRowView[] {
  return dashboard.recentCorrections.map((correction) => ({
    id: correction.id,
    title: `${formatTimestampDate(correction.createdAt)} · ${correction.tenantName}`,
    subject: correctionSubject(correction),
    amount: formatSignedRupiah(correction.amountDelta),
    reason: correction.reason,
    by: `oleh ${correction.createdBy.name}`,
  }));
}

// Beranda's money shows only to a member who may read reports, on some records or all (a member
// limited to some properties gets their properties' figures). Never by role name.
export function canReadDashboard(ability: AppAbility): boolean {
  return ability.can("read", "report");
}

// A tenant's name links to Detail penyewa for a member who may read tenants.
export function canOpenTenant(ability: AppAbility): boolean {
  return ability.can("read", "tenant");
}
