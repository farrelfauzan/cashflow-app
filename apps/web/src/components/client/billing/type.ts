import type {
  ChoiceOptionView,
  PromiseDraft,
  PromiseOfferView,
} from "@/components/client/payment-page/type";
import type { ChipTone } from "@/components/client/tenant/type";

// The tenant's money on the Detail penyewa page (board "iPad · Detail penyewa"). Every value is
// ready to show; every amount in it is a field the API sent, only formatted.

// One summary tile: "Tunggakan", "Tunggakan tertua", "Janji aktif", "Saldo lebih bayar".
export type AccountTileView = {
  label: string;
  value: string;
  // Small lines under the value: "sisa November", "Total belum dibayar Rp1.500.000".
  notes: readonly string[];
  // "warning" while the tenant has debt (Tunggakan above zero).
  tone: "default" | "warning";
};

// The chip of an invoice: "Lunas", "Sebagian", "Belum jatuh tempo", "Terlambat 3 hari",
// "Dibatalkan".
export type InvoiceStatusView = {
  label: string;
  tone: ChipTone;
};

export type InvoiceLineView = {
  id: string;
  label: string;
  amount: string;
};

// One invoice in "Tagihan", newest period first.
export type InvoiceRowView = {
  id: string;
  // The month of the period: "November 2026".
  title: string;
  // The unit and property, only when the tenant's invoices come from more than one lease.
  place: string | null;
  // "jatuh tempo 1 Des".
  due: string;
  // What it bills now (billed), or for a cancelled invoice what it first billed, struck through.
  amount: string;
  struck: boolean;
  // "dikoreksi dari Rp1.500.000" when a correction changed what it bills; null otherwise.
  correctedFrom: string | null;
  status: InvoiceStatusView;
  // "dibayar Rp250.000", "Sisa Rp1.250.000", "2 pembayaran".
  details: readonly string[];
  lines: readonly InvoiceLineView[];
  // The name of its "Koreksi" action and of the dialog it opens: "Koreksi tagihan November 2026".
  correctionTitle: string;
};

export type AllocationView = {
  invoiceId: string;
  // "→ Oktober Rp750.000 (lunas)".
  text: string;
};

// One payment in "Pembayaran dan alokasi", newest first.
export type PaymentRowView = {
  id: string;
  // "25 Nov 2026 · Tunai".
  title: string;
  // What it counts for now (net).
  amount: string;
  correctedFrom: string | null;
  note: string | null;
  // "dicatat Mbak Sari Wulandari", null when the API names nobody.
  recordedBy: string | null;
  // Empty when it all went to credit.
  allocations: readonly AllocationView[];
  // The name of its "Koreksi" action and of the dialog it opens: "Koreksi pembayaran 25 Nov 2026".
  correctionTitle: string;
};

// One correction in "Koreksi nominal", newest first.
export type CorrectionRowView = {
  id: string;
  // "2 Nov 2026 · Tagihan November 2026" or "2 Nov 2026 · Pembayaran 25 Nov 2026".
  title: string;
  // A second line naming what was corrected, when the title does not (Beranda's list:
  // "Tagihan November 2026 · Kamar 11, Kos Melati").
  subject?: string;
  // Signed: "−Rp50.000", "+Rp100.000".
  amount: string;
  reason: string;
  // "oleh Pak Hendra Gunawan".
  by: string;
};

// What "Koreksi" was pressed on: one invoice or one payment of the tenant, by id. The dialog reads
// the record from the latest account, so a refetch while it is open shows the current amount.
export type CorrectionSelection = {
  kind: "invoice" | "payment";
  id: string;
};

// The record a correction is sent for: exactly one of the two ids (CorrectionInput).
export type CorrectionTarget = { invoiceId: string } | { paymentId: string };

// The "Koreksi nominal" dialog (board "iPad · Koreksi nominal") for one invoice or payment.
export type CorrectionDialogView = {
  target: CorrectionTarget;
  // "Koreksi tagihan November 2026", "Koreksi pembayaran 25 Nov 2026".
  title: string;
  // "Yulia Kartika · Kamar 3 · Kos Melati" for an invoice, the tenant's name for a payment.
  subtitle: string;
  // What it bills or counts now (billed or net), formatted: "Rp1.500.000".
  current: string;
};

// A request from outside the section to open "Koreksi nominal" for one invoice (from "Catat
// meteran"'s locked view). Each request has its own id, so asking twice opens the dialog twice.
export type CorrectionRequest = {
  id: number;
  invoiceId: string;
};

// The chip of a promise: "Aktif", "Ditepati", "Ditepati sebagian", "Tidak ditepati".
export type PromiseStatusView = {
  label: string;
  tone: ChipTone;
};

// One promise in "Riwayat janji", newest first, from its newest version.
export type PromiseHistoryRowView = {
  id: string;
  // "Sab, 12 Des · Rp1.250.000" or "Sab, 12 Des · Semua sisa tagihan".
  title: string;
  // "dibuat penyewa 25 Nov" or "dicatat Pak Hendra Gunawan 6 Nov", with " · diubah 1×" after an
  // edit.
  byline: string;
  status: PromiseStatusView;
};

// What "Catat janji" holds: the promise's fields as the tenant's Janji form has them, plus the
// member's note.
export type StaffPromiseValues = PromiseDraft & {
  note: string;
};

// The "Catat janji" dialog, made from the account when it opens.
export type PromiseDialogView = {
  // The tenant's name.
  subtitle: string;
  // "Janji aktif Sab, 12 Des, Rp1.250.000 akan diganti." while the tenant has an active promise;
  // null otherwise.
  replacesLine: string | null;
  // What a promise made now may name, from the account.
  offer: PromiseOfferView;
  // "Bareng tagihan berikutnya" (when offered) and "Pilih tanggal sendiri".
  kindOptions: ChoiceOptionView[];
  // The date input's bounds (YYYY-MM-DD).
  dateMin: string;
  dateMax: string;
  // "Kosongkan untuk semua sisa, Rp1.250.000."
  amountHint: string;
  // "Simpan perubahan" over an active promise, else "Simpan janji".
  primaryLabel: string;
  // The fields as the dialog opens: the active promise's, or empty.
  defaults: StaffPromiseValues;
};

// The "Catat janji" dialog on the page: its view, frozen when it opened, whether it shows, and
// which opening it is (each opening mounts a fresh form).
export type PromiseDialogState = {
  view: PromiseDialogView;
  open: boolean;
  opening: number;
};
