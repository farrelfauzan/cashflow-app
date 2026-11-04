import type { AccountTileView } from "@/components/client/billing/type";
import { FINAL_INVOICE_LABEL } from "@/components/client/billing/utils";
import type { SortOptions } from "@/components/client/list/type";
import {
  readChoice,
  readSearch,
  SEARCH_PARAM,
  searchQuery,
} from "@/components/client/list/utils";
import type { FilterOption } from "@/components/client/tenant/type";
import {
  PROPERTY_PARAM,
  STATUS_PARAM,
  UUID_PATTERN,
} from "@/components/client/tenant/utils";
import { ApiError } from "@/lib/api/errors";
import {
  ListInvoicesSort,
  ListInvoicesStatus,
  type InvoiceListItem,
  type InvoiceListSummary,
  type MissingMeterReading,
} from "@/lib/api/generated/model";
import { formatDate, formatDayMonth, formatMonthYear } from "@/lib/format/date";
import { formatRupiah } from "@/lib/format/rupiah";

import type {
  InvoiceChip,
  InvoiceListFilters,
  InvoiceRowView,
  MissingMeterRowView,
} from "./type";

// Penagihan: the month's invoices. Every amount shown is a field the API sent, only formatted.
// The one ratio is the "N%" of the first tile; counting and calendar arithmetic on months are
// the only other computations. Whether an invoice is overdue is the API's (overdueDays), never
// a date compared in the browser.

export const SEARCH_LABEL = "Cari penyewa atau unit…";
export const LOADING_TEXT = "Memuat…";
export const FORBIDDEN_TEXT = "Anda tidak punya akses ke penagihan.";
export const NO_MATCH_TITLE = "Tidak ada tagihan yang cocok.";
export const NO_MATCH_HINT = "Coba kata lain, atau pilih Semua status.";
export const CLEAR_FILTERS_LABEL = "Hapus filter";
export const ALL_MONTHS_TEXT = "Semua tunggakan, semua bulan";
export const PREVIOUS_MONTH_LABEL = "Bulan sebelumnya";
export const NEXT_MONTH_LABEL = "Bulan berikutnya";
export const LIST_LABEL = "Daftar tagihan";

export const PERIOD_PARAM = "period";

// The iPad table's columns, shared by its header and its rows: Penyewa, Unit (the widest free
// column, so "Kontrakan Mawar" stays on one line), Tagihan, Dibayar, Sisa, and Status with
// "Jatuh tempo 30 Des 2026" under the chip.
export const TABLE_COLUMNS =
  "grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)_108px_108px_108px_188px]";

export const STATUS_FILTER_OPTIONS: readonly FilterOption[] = [
  { value: ListInvoicesStatus.unpaid, label: "Belum lunas" },
  { value: ListInvoicesStatus.overdue, label: "Terlambat" },
  { value: ListInvoicesStatus.open, label: "Belum dibayar" },
  { value: ListInvoicesStatus.partial, label: "Sebagian" },
  { value: ListInvoicesStatus.paid, label: "Lunas" },
  { value: ListInvoicesStatus.cancelled, label: "Dibatalkan" },
];

// The sort choices; the API's default (the oldest due date) comes first.
export const INVOICE_SORT_OPTIONS: SortOptions<ListInvoicesSort> = [
  { value: ListInvoicesSort.dueDate, label: "Jatuh tempo terlama" },
  { value: ListInvoicesSort["-dueDate"], label: "Jatuh tempo terbaru" },
  { value: ListInvoicesSort.tenantName, label: "Nama penyewa" },
  { value: ListInvoicesSort["-outstanding"], label: "Sisa terbesar" },
];

const INVOICE_STATUSES = Object.values(ListInvoicesStatus);

// Takes the search, the property and the status off in one change ("Hapus filter"). The month
// stays: it is where the member is looking, not a filter.
export const CLEAR_FILTERS: Readonly<Record<string, null>> = {
  [SEARCH_PARAM]: null,
  [PROPERTY_PARAM]: null,
  [STATUS_PARAM]: null,
};

const PERIOD_PATTERN = /^(\d{4})-(0[1-9]|1[0-2])$/;

// The month in the URL when it is a real YYYY-MM; anything else counts as not set, so a
// hand-edited link shows this month and the API never answers 400.
export function readPeriod(params: URLSearchParams): string | undefined {
  const raw = params.get(PERIOD_PARAM);
  return raw !== null && PERIOD_PATTERN.test(raw) ? raw : undefined;
}

// This month in Jakarta, YYYY-MM, from the Jakarta day (YYYY-MM-DD).
export function currentPeriod(today: string): string {
  return today.slice(0, 7);
}

function shiftMonth(period: string, months: number): string {
  const match = PERIOD_PATTERN.exec(period);
  if (match === null) {
    throw new RangeError(`expected YYYY-MM, got ${JSON.stringify(period)}`);
  }
  const index = Number(match[1]) * 12 + (Number(match[2]) - 1) + months;
  const year = Math.floor(index / 12);
  const month = (index % 12) + 1;
  return `${String(year).padStart(4, "0")}-${String(month).padStart(2, "0")}`;
}

// "2026-11" gives "2026-10"; January goes back to the December before it.
export function previousMonth(period: string): string {
  return shiftMonth(period, -1);
}

export function nextMonth(period: string): string {
  return shiftMonth(period, 1);
}

// "November 2026".
export function monthLabel(period: string): string {
  return formatMonthYear(`${period}-01`);
}

// The month being looked at: the one in the URL, else this month.
export function shownPeriod(
  period: string | undefined,
  thisMonth: string,
): string {
  return period ?? thisMonth;
}

// "Bulan berikutnya" goes no further than this month (YYYY-MM compares as a string).
export function isLatestMonth(period: string, thisMonth: string): boolean {
  return period >= thisMonth;
}

// A month as a URL param: this month is the default and stays out of the URL.
export function periodParam(period: string, thisMonth: string): string | null {
  return period === thisMonth ? null : period;
}

// The invoice filters in the URL. A status outside the API's list, a property that is not an id
// or a month that is not YYYY-MM counts as not set.
export function readInvoiceFilters(
  params: URLSearchParams,
): InvoiceListFilters {
  const propertyId = params.get(PROPERTY_PARAM) ?? "";
  return {
    period: readPeriod(params),
    q: searchQuery(readSearch(params)),
    propertyId: UUID_PATTERN.test(propertyId) ? propertyId : undefined,
    status: readChoice(params, STATUS_PARAM, INVOICE_STATUSES),
  };
}

// What the request carries: debt of every month ("overdue") has no month, so none is sent.
export function listPeriod(filters: InvoiceListFilters): string | undefined {
  return filters.status === ListInvoicesStatus.overdue
    ? undefined
    : filters.period;
}

// Whether the search or a filter narrows the list: then an empty answer is "Tidak ada tagihan
// yang cocok", not "Belum ada tagihan bulan ini".
export function isFiltering(filters: InvoiceListFilters): boolean {
  return (
    filters.q !== undefined ||
    filters.propertyId !== undefined ||
    filters.status !== undefined
  );
}

// "Belum ada tagihan bulan ini." for this month, and the month's name for another.
export function noInvoicesText(period: string, thisMonth: string): string {
  return period === thisMonth
    ? "Belum ada tagihan bulan ini."
    : `Belum ada tagihan di ${monthLabel(period)}.`;
}

// A forbidden screen: the member lacks invoice:read (the API's 403 for what the menu hid).
export function isForbidden(error: unknown): boolean {
  return error instanceof ApiError && error.status === 403;
}

// The share of the billed amount that is collected, whole percent: the one ratio the screen
// computes, as on Beranda. Capped, so a figure the API sends out of range never shows 120%.
export function paidPercent(paid: number, billed: number): number {
  if (!(billed > 0)) return 0;
  return Math.min(100, Math.max(0, Math.floor((paid * 100) / billed)));
}

// The three tiles above the list, from the summary only. They cover the whole month in the
// chosen property, whatever the search, status and page are. The oldest debt is not tied to the
// month.
export function summaryTiles(
  summary: InvoiceListSummary,
  period: string,
  thisMonth: string,
): AccountTileView[] {
  const collectedNote =
    summary.billed === 0
      ? noInvoicesText(period, thisMonth)
      : `${paidPercent(summary.paid, summary.billed)}% dari ${formatRupiah(summary.billed)}`;
  const outstandingNote =
    summary.outstanding === 0 && summary.billed > 0
      ? "Semua sudah lunas"
      : `${summary.tenantsOutstanding} penyewa`;
  const oldest = summary.oldestOverdue;
  return [
    {
      label: "Terkumpul",
      value: formatRupiah(summary.paid),
      notes: [collectedNote],
      tone: "default",
    },
    {
      label: "Sisa belum dibayar",
      value: formatRupiah(summary.outstanding),
      notes: [outstandingNote],
      tone: "default",
    },
    oldest === undefined
      ? {
          label: "Tunggakan tertua",
          value: "Tidak ada",
          notes: ["Tidak ada tunggakan"],
          tone: "default",
        }
      : {
          label: "Tunggakan tertua",
          value: `Terlambat ${oldest.ageDays} hari`,
          notes: [
            `${oldest.tenantName} · ${oldest.unitLabel}, ${oldest.propertyName}`,
          ],
          tone: "warning",
        },
  ];
}

export const PAST_PLANNED_END_CHIP = "Lewat rencana keluar";

// The status chip, from the API's fields: overdueDays is the API's own count (0 on the due date
// itself, since debt starts the day after), and pastPlannedEnd adds a second chip. The final
// invoice (made at move-out for metered usage) says so in a chip of its own, since the list
// names no month per row.
export function invoiceChips(
  invoice: Pick<
    InvoiceListItem,
    "kind" | "status" | "overdueDays" | "pastPlannedEnd"
  >,
): InvoiceChip[] {
  const chips: InvoiceChip[] = [statusChip(invoice)];
  if (invoice.kind === "final") {
    chips.push({ label: FINAL_INVOICE_LABEL, tone: "neutral" });
  }
  if (invoice.pastPlannedEnd) {
    chips.push({ label: PAST_PLANNED_END_CHIP, tone: "warning" });
  }
  return chips;
}

function statusChip(
  invoice: Pick<InvoiceListItem, "status" | "overdueDays">,
): InvoiceChip {
  switch (invoice.status) {
    case "paid":
      return { label: "Lunas", tone: "active" };
    case "cancelled":
      return { label: "Dibatalkan", tone: "ended" };
    case "open":
    case "partial":
      if (invoice.overdueDays > 0) {
        return {
          label: `Terlambat ${invoice.overdueDays} hari`,
          tone: "warning",
        };
      }
      return invoice.status === "partial"
        ? { label: "Sebagian", tone: "neutral" }
        : { label: "Belum jatuh tempo", tone: "upcoming" };
  }
}

export function invoiceRow(invoice: InvoiceListItem): InvoiceRowView {
  return {
    id: invoice.id,
    tenantId: invoice.tenantId,
    tenantName: invoice.tenantName,
    unit: invoice.unitLabel,
    property: invoice.propertyName,
    dueDate: formatDate(invoice.dueDate),
    billed: formatRupiah(invoice.billed),
    paid: formatRupiah(invoice.paid),
    outstanding: formatRupiah(invoice.outstanding),
    chips: invoiceChips(invoice),
  };
}

// Where the invoice's unit is, as the phone card reads it: "Kamar 3 · Kos Melati".
export function unitText(
  row: Pick<InvoiceRowView, "unit" | "property">,
): string {
  return `${row.unit} · ${row.property}`;
}

// "Meteran belum dicatat" (boards W14 Penagihan): the leases that expect a metered amount (listrik,
// air) for last month and have none. The API counts and lists them (listMissingMeterReadings);
// the screen only words what it sends.

// The URL param that swaps the invoices for that list, and its one value. It is a different view,
// not a filter of invoices: the month param stays as it is.
export const METER_PARAM = "meter";
export const METER_MISSING = "missing";

export const METER_FLAG_HINT =
  "Tagihan tetap terbit tanpa listrik atau air. Yang dicatat terlambat masuk ke tagihan bulan berikutnya.";
export const METER_SHOW_LABEL = "Tampilkan";
export const METER_CHIP_LABEL = "Meteran belum dicatat";
export const METER_CHIP_CLEAR_LABEL = "Hapus filter Meteran belum dicatat";
export const METER_LIST_LABEL = "Sewa dengan meteran belum dicatat";
export const METER_ALL_RECORDED_TEXT = "Semua meteran sudah dicatat.";
export const TENANT_LINK_LABEL = "Lihat penyewa";

// The iPad table's columns, shared by its header and its rows: Penyewa, Unit, Belum dicatat,
// Tagihan berikutnya and Aksi.
export const METER_TABLE_COLUMNS =
  "grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_minmax(0,1.3fr)_150px_140px]";

// Whether the URL asks for the list of leases with a meter amount missing. Any other value of
// the param counts as not set, so a hand-edited link shows the normal Penagihan.
export function readMeterView(params: URLSearchParams): boolean {
  return params.get(METER_PARAM) === METER_MISSING;
}

// Last month in Jakarta, YYYY-MM, from the Jakarta day (YYYY-MM-DD): the month the API counts by
// default. January goes back to the December before it.
export function lastMonthPeriod(today: string): string {
  return previousMonth(currentPeriod(today));
}

// "Oktober 2026".
export function lastMonthLabel(today: string): string {
  return monthLabel(lastMonthPeriod(today));
}

// The flag's bold line: "Meteran Oktober 2026 belum dicatat untuk 3 sewa".
// The line under the title in place of the month stepper, or null for the stepper: the missing
// meter readings are always last month's, and overdue invoices have no month.
export function headerLine(
  meterView: boolean,
  allMonths: boolean,
  today: string,
): string | null {
  if (meterView) return `Meteran ${lastMonthLabel(today)}`;
  if (allMonths) return ALL_MONTHS_TEXT;
  return null;
}

export function meterFlagTitle(today: string, total: number): string {
  return `Meteran ${lastMonthLabel(today)} belum dicatat untuk ${total} sewa`;
}

// "Menampilkan 3 sewa", under the chip.
export function meterShowingText(total: number): string {
  return `Menampilkan ${total} sewa`;
}

// "Listrik, Air": the types with no amount yet, in the API's order.
export function missingNames(missing: MissingMeterReading["missing"]): string {
  return missing.map((type) => type.name).join(", ");
}

// "terbit 8 Nov", or "—" when the lease has no next invoice to name.
export function nextInvoiceText(
  nextInvoice: MissingMeterReading["nextInvoice"],
): string {
  return nextInvoice === undefined
    ? "—"
    : `terbit ${formatDayMonth(nextInvoice.issuedOn)}`;
}

export function missingMeterRow(
  item: MissingMeterReading,
): MissingMeterRowView {
  return {
    leaseId: item.leaseId,
    tenantId: item.tenantId,
    tenantName: item.tenantName,
    unit: item.unitLabel,
    property: item.propertyName,
    missing: missingNames(item.missing),
    nextInvoice: nextInvoiceText(item.nextInvoice),
  };
}
