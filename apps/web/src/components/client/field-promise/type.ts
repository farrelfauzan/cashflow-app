import type { FieldTodayItem } from "@/lib/api/generated/model";

// The tenant a "Catat janji bayar" sheet is for: the fields of their Tagihan hari ini row it
// uses. A whole FieldTodayItem fits.
export type FieldPromiseTenant = Pick<
  FieldTodayItem,
  "tenantId" | "tenantName" | "outstanding"
>;
