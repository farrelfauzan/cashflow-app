import type { z } from "zod";

import type { DepositChip } from "@/components/client/deposit/type";

import type { countFormSchema } from "./utils";

// "Hitung setoran" as react-hook-form holds it, under the API's field names.
export type CountFormValues = z.input<typeof countFormSchema>;

// What the schema hands on: the integer amount counted and the trimmed note.
export type CountFormOutput = z.output<typeof countFormSchema>;

// The two tabs of "Setoran tunai": not counted yet, and counted.
export type DepositTab = "pending" | "confirmed";

// What a deposit's row offers: "Hitung", the muted "Dihitung admin lain" on the member's own
// deposit, or nothing for a member who may only read (the Supervisor).
export type CountAction = "count" | "own" | "none";

// A deposit waiting to be counted, ready to show (tab "Menunggu dihitung").
export type PendingDepositRowView = {
  id: string;
  staffName: string;
  // "Disetor Sabtu 14 Nov, 16.20".
  recordedLine: string;
  paymentCount: number;
  // The payments' total (expectedAmount) and what the karyawan says they handed over (amount).
  expected: string;
  handed: string;
  // "kurang Rp50.000" or "lebih Rp50.000" when the two differ; null when they are the same.
  handedNote: string | null;
  action: CountAction;
};

// A counted deposit, ready to show (tab "Sudah dihitung").
export type CountedDepositRowView = {
  id: string;
  staffName: string;
  // "Disetor Sab 14 Nov, 16.20".
  recordedLine: string;
  expected: string;
  // "3 kwitansi".
  receipts: string;
  received: string;
  result: DepositChip;
  // Under a gap's chip: "Di Antrean cek" or "Selisih ditutup"; null for "Cocok".
  resultNote: string | null;
  countedBy: string;
  // "Sab 14 Nov, 17.02".
  countedAt: string;
};

// The top of the "Hitung setoran" drawer.
export type CountSheetView = {
  // "Mas Dimas Saputra · disetor Sabtu 14 Nov, 16.20".
  subtitle: string;
  expected: string;
  // "3 kwitansi".
  receipts: string;
  handed: string;
  // The photo of the money, as text: "Foto belum ada".
  photo: string;
};

// One payment the deposit covers, in the drawer.
export type DepositReceiptView = {
  paymentId: string;
  tenantName: string;
  // "Rumah A1 · Jum 13 Nov, 10.15"; the time alone when the tenant has no unit.
  detail: string;
  amount: string;
  photo: string;
};

// The live line under "Uang yang Anda terima", against the payments' total. A hint only: the
// saved gap is the API's.
export type CountHint = {
  tone: "match" | "gap";
  text: string;
};

// How the drawer answers a refused count: someone counted it first (close, reload), or show the
// API's message the way every form does.
export type ConfirmErrorKind = "already_confirmed" | "other";
