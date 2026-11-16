import type { ChipTone } from "@/components/client/tenant/type";
import type { FieldTodayItem } from "@/lib/api/generated/model";

// A chip's text and look: why the tenant is on the list, or the photo of today's cash.
export type FieldChip = {
  label: string;
  tone: ChipTone;
};

// "Tunai hari ini Rp950.000" with the photo's chip, under a tenant the karyawan took cash from
// today.
export type FieldCashLineView = {
  text: string;
  photo: FieldChip;
};

// One tenant on Tagihan hari ini (FieldTodayItem), ready to show.
export type FieldTodayRowView = {
  tenantId: string;
  tenantName: string;
  // "Kamar 11 · Kos Melati", several units joined by ", "; empty without one.
  place: string;
  // "Sisa Rp1.050.000 · telat Rp1.050.000", "Sisa Rp1.050.000" or "Lunas".
  owed: string;
  reason: FieldChip;
  cash: FieldCashLineView | null;
  // The API's row, for the sheets it opens ("Terima tunai", "Catat janji bayar").
  item: FieldTodayItem;
  // Still owes something: the row offers "Terima tunai" and "Catat janji bayar". A tenant who paid everything today
  // ("Lunas") shows no action (board HP-Tagihan).
  canCollect: boolean;
};
