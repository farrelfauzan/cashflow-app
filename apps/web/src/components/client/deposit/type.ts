import type { z } from "zod";

import type { ChipTone } from "@/components/client/tenant/type";

import type { depositFormSchema } from "./utils";

// "Setor tunai" (board HP-Setor) as react-hook-form holds it, under the API's field name.
export type DepositFormValues = z.input<typeof depositFormSchema>;

// What the schema hands on: the integer amount, ready for the request body.
export type DepositFormOutput = z.output<typeof depositFormSchema>;

// A chip's text and look: the photo of a receipt, the result of a counted deposit.
export type DepositChip = {
  label: string;
  tone: ChipTone;
};

// One receipt the karyawan holds (HeldCashPayment), ready to show.
export type HeldReceiptView = {
  paymentId: string;
  tenantName: string;
  // "Rumah A1 · Jumat 13 Nov, 10.15"; the date alone when the tenant has no unit.
  detail: string;
  amount: string;
  photo: DepositChip;
};

// A deposit waiting for an admin to count it (board phone 2).
export type PendingDepositView = {
  id: string;
  amount: string;
  // "Disetor Sabtu 14 Nov, 16.20 · 3 kwitansi".
  detail: string;
};

// A counted deposit under "Setoran sebelumnya".
export type PastDepositView = {
  id: string;
  // "Kamis, 12 Nov".
  date: string;
  // "Dihitung Pak Budi · 4 kwitansi", or for a gap "Disetor Rp2.000.000 · dihitung Rp1.950.000".
  detail: string;
  // What the admin counted.
  amount: string;
  chip: DepositChip;
};

// How the screen answers a refused deposit: load the cash again (deposit_cash_changed,
// nothing_to_deposit), or show the API's message the way every form does.
export type DepositErrorKind =
  "cash_changed" | "nothing_to_deposit" | "request_reused" | "other";
