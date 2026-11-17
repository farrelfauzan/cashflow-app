import type { FieldTodayItem } from "@/lib/api/generated/model";

// The tenant a "Catat janji bayar" sheet is for: the fields of their Tagihan hari ini row it
// uses. A whole FieldTodayItem fits. What they owe comes from the offer the sheet reads.
export type FieldPromiseTenant = Pick<
  FieldTodayItem,
  "tenantId" | "tenantName"
>;
