import type { LucideIcon } from "lucide-react";
import type { z } from "zod";

import type { DepositChip } from "@/components/client/deposit/type";
import type { CheckQueueKind } from "@/lib/api/generated/model";

import type { dismissFormSchema } from "./utils";

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
  // Whether the row opens a drawer; a transfer proof does not yet (its drawer comes later).
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

// The two states of the "Nomor baru" drawer: deciding, and saying why the number is not used.
export type PhoneSheetStep = "review" | "reject";

// How a drawer answers a refused action: someone answered the item first (close, reload), the
// new number belongs to another tenant (under the number), or show the API's message as every
// form does.
export type CheckActionErrorKind = "resolved" | "phone_taken" | "other";

// What else an answered item changed, besides the queue.
export type CheckRefreshScope = {
  // A closed deposit gap: the deposit now shows "Selisih ditutup".
  deposits?: boolean;
  // An applied number: the tenant's phone changed.
  tenantId?: string;
};
