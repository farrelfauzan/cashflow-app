import type { ChipTone } from "@/components/client/tenant/type";

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
};
