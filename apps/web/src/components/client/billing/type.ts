import type { ChipTone } from "@/components/client/tenant/type";

// The tenant's money on the Detail penyewa page (board "iPad · Detail penyewa"). Every value is
// ready to show; every amount in it is a field the API sent, only formatted.

// One summary tile: "Tunggakan", "Tunggakan tertua", "Saldo lebih bayar".
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
