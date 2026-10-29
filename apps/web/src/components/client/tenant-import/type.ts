import type { ImportResult } from "@/lib/api/generated/model";

// The three steps of "Impor penyewa", held in the screen's state. The file's text stays in memory
// only, so the commit sends exactly what was checked; it is never logged or stored.
export type ImportStep =
  | { kind: "choose" }
  | {
      kind: "check";
      fileName: string;
      text: string;
      result: ImportResult;
      // The commit came back with committed false: the data changed since the check.
      changed: boolean;
    }
  | { kind: "done"; fileName: string; result: ImportResult };

export type StepNumber = 1 | 2 | 3;

// Why step 1 cannot go on with the chosen file.
export type ChooseProblem =
  // Over 1 MiB: refused before any request.
  | { kind: "tooLarge" }
  // The API answered 400: the file cannot be read (missing columns, no rows, too many).
  | { kind: "unreadable"; message: string }
  // Any other failure, with the API's message. `chooseOther` true asks for another file (a
  // denial, a file the API refuses as too large); false keeps "Periksa berkas" (a network error).
  | { kind: "alert"; message: string; chooseOther: boolean };

// The action at the bottom of step 1.
export type ChooseAction = "checkDisabled" | "check" | "chooseOther";

export type RowFilter = "all" | "errors" | "notes";

export type FilterChipView = {
  filter: RowFilter;
  label: string;
};

export type RowStatusTone = "ready" | "matched" | "error";

// A row's chips: its status, or (neutral) something the row will create.
export type RowChipTone = RowStatusTone | "neutral";

// One line under a row: a problem ("Kolom no_wa: ...") or a note ("Catatan: ..."). `label` is the
// bold lead, null for an error the API does not tie to a column.
export type RowMessageView = {
  label: string | null;
  text: string;
};

export type ImportRowView = {
  line: number;
  lineLabel: string;
  status: { label: string; tone: RowStatusTone };
  tenantName: string;
  // "Kamar 1 · Kos Melati · Rp1.250.000"
  placeText: string;
  // "Mulai 1 Okt 2026, tagih dari 1 Nov 2026"; null when the API could not read the terms.
  datesText: string | null;
  // What the row will create: "Properti baru", "Unit baru", "Penyewa baru", "Sewa baru".
  createdChips: string[];
  errors: RowMessageView[];
  notes: RowMessageView[];
};

export type SummaryTileView = {
  label: string;
  value: number;
  // "Perlu diperbaiki" above 0.
  warning: boolean;
};

export type CommitButtonView = {
  label: string;
  enabled: boolean;
  hint: string;
};

export type CountRowView = {
  label: string;
  value: number;
};
