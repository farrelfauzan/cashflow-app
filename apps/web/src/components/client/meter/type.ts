import type { z } from "zod";

import type { meterFormSchema } from "./utils";

// "Catat meteran" (board "W14 iPad · Catat meteran") as react-hook-form holds it, under the API's
// field names: the usage month and charge type go in the request path, the rest in the body.
export type MeterFormValues = z.input<typeof meterFormSchema>;

// What the schema hands on: the checked fields, integers where the API wants them.
export type MeterFormOutput = z.output<typeof meterFormSchema>;

// One choice of "Bulan pemakaian": the usage month (YYYY-MM) and "Oktober 2026".
export type MonthOption = {
  value: string;
  label: string;
};

// The month and charge type the form is on; one recorded amount at most matches it.
export type MeterSelection = {
  usageMonth: string;
  chargeTypeId: string;
};

// One amount in "Sudah dicatat", newest month first.
export type RecordedMeterRowView = {
  id: string;
  selection: MeterSelection;
  // "Listrik · Oktober 2026 · Rp123.000".
  text: string;
  // "Listrik Oktober 2026", the name of its "Ubah" action.
  name: string;
  // Billed amounts are locked: the row opens the locked view instead of the form.
  locked: boolean;
};

// One row of the locked view: "Jumlah", "Angka awal", "Angka akhir", "Dicatat".
export type LockedMeterRow = {
  label: string;
  value: string;
};

// The locked view (board "Meteran sudah ditagihkan"), every value ready to show.
export type LockedMeterView = {
  // "Listrik · Oktober 2026".
  heading: string;
  rows: readonly LockedMeterRow[];
  text: string;
  // The invoice that bills it, for "Koreksi tagihan November 2026".
  invoiceId: string;
  correctLabel: string;
};
