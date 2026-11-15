import type { LucideIcon } from "lucide-react";
import type { z } from "zod";

import type { DepositChip } from "@/components/client/deposit/type";
import type { CheckQueueKind } from "@/lib/api/generated/model";

import type { dismissFormSchema, proofMatchSchema } from "./utils";

// A note form (closing an item, or why a new number is not used) as react-hook-form holds it,
// under the API's field name.
export type NoteFormValues = z.input<typeof dismissFormSchema>;

// What the schema hands on: the trimmed note.
export type NoteFormOutput = z.output<typeof dismissFormSchema>;

// The look of a row's icon tile: the accent tint, or the warning tint for what may have moved
// money the wrong way (a deposit gap, an infected file, a QRIS mismatch).
export type CheckIconTone = "accent" | "warning";

// One waiting item, ready to show as a row of the list.
export type CheckRowView = {
  id: string;
  kind: CheckQueueKind;
  // "Bukti transfer"; shown in capitals.
  kindLabel: string;
  icon: LucideIcon;
  iconTone: CheckIconTone;
  // "Bu Wati Handayani · Kamar 11, Kos Melati".
  title: string;
  // "Foto bukti dari halaman pembayaran".
  detail: string;
  chip: DepositChip;
  // "Masuk Sabtu 14 Nov, 17.02".
  arrived: string;
  // Whether the row opens a drawer; every kind has one.
  opens: boolean;
};

// One choice of the filter row: every kind (null) or one, with how many wait (once known).
export type CheckFilterOption = {
  kind: CheckQueueKind | null;
  label: string;
  count: number | undefined;
};

// A figure tile in a drawer: "Total kwitansi" Rp2.500.000 "3 kwitansi".
export type CheckTile = {
  label: string;
  value: string;
  // The small line under the value; null for none.
  sub: string | null;
  tone: "plain" | "warning" | "accent";
};

// The kind of note box in a drawer: the mint information callout, or the warning one.
export type CheckCalloutTone = "info" | "warning";

// "Selisih setoran".
export type DepositGapSheetView = {
  subtitle: string;
  tiles: CheckTile[];
  todo: string;
};

// "File ditolak".
export type UploadInfectedSheetView = {
  subtitle: string;
  // "Diperiksa Sabtu 14 Nov, 11.07 · sudah diamankan".
  scannedLine: string;
  todo: string;
};

// "QRIS tidak cocok".
export type QrisMismatchSheetView = {
  subtitle: string;
  tiles: CheckTile[];
  callout: string;
  transactionId: string;
  tenantId: string;
  // "Buka Pak Joko Susilo".
  tenantLinkText: string;
};

// "Nomor baru".
export type PhoneChangeSheetView = {
  subtitle: string;
  tiles: CheckTile[];
  // "Catatan Mas Dimas Saputra · Sabtu 14 Nov, 17.02" and the note, when the karyawan wrote one.
  noteCaption: string | null;
  note: string | null;
  // "Nomor Bu Siti Aminah tetap 0812-3456-7890. Item ini hilang dari antrean."
  keepLine: string;
  applyToast: string;
};

// The two states of the "Nomor baru" and "Bukti transfer" drawers: deciding, and saying why the
// item is not used.
export type PhoneSheetStep = "review" | "reject";
export type ProofSheetStep = PhoneSheetStep;

// How a drawer answers a refused action: someone answered the item first (close, reload), the
// new number belongs to another tenant (under the number), the proof's photo is still being
// scanned (show it as pending), the scan refused the photo (close, reload: it is now a "File
// ditolak" item), or show the API's message as every form does.
export type CheckActionErrorKind =
  "resolved" | "phone_taken" | "upload_not_ready" | "upload_refused" | "other";

// "Bukti transfer", matched as a payment ("Simpan pembayaran"), as react-hook-form holds it under
// the API's field names: the amount as typed (digits grouped with dots), the day, the note.
export type ProofMatchValues = z.input<ReturnType<typeof proofMatchSchema>>;

// What the schema hands on: the integer amount, the checked day, the trimmed note.
export type ProofMatchOutput = z.output<ReturnType<typeof proofMatchSchema>>;

// The proof's photo as the drawer shows it: still being scanned, loading its link, ready at a
// short-lived link, refused by the scan (the drawer closes), or not readable (the API's message).
export type ProofPhotoState =
  | { kind: "pending" }
  | { kind: "loading" }
  | { kind: "clean"; url: string }
  | { kind: "refused" }
  | { kind: "error"; message: string };

// What stands beside the photo: when the tenant sent it ("Jum 13 Nov, 19.42", under "Dikirim
// penyewa") and, when the tenant's account names the link's invoice, its month ("November 2026",
// under "Dari tautan tagihan").
export type ProofPhotoCaption = {
  sentAt: string;
  link: string | null;
};

// "Pembayaran dari": the tenant and "Kamar 11, Kos Melati · sisa Rp1.050.000" (the unit or the
// remainder left out when unknown).
export type ProofPayerView = {
  name: string;
  detail: string | null;
};

// One invoice the typed amount would reach, from the API's preview: "Tagihan November 2026 jadi
// Lunas", or "Tagihan Desember 2026: Sebagian, sisa Rp50.000".
export type ProofPreviewLine = {
  id: string;
  lead: string;
  result: string;
};

// "Setelah disimpan:" and what the API says the amount would pay; every figure is the API's.
export type ProofPreviewView = {
  // The amount previewed, as the API answered it.
  amount: number;
  lines: ProofPreviewLine[];
  // "Kelebihan Rp50.000 jadi saldo", or null when no credit is added.
  credit: string | null;
};

// What else an answered item changed, besides the queue.
export type CheckRefreshScope = {
  // A closed deposit gap: the deposit now shows "Selisih ditutup".
  deposits?: boolean;
  // An applied number: the tenant's phone changed.
  tenantId?: string;
};
