import { ApiError, GENERIC_ERROR_MESSAGE } from "@/lib/api/errors";
import { API_BASE_PATH } from "@/lib/api/client";
import type { ImportRow, ImportSummary } from "@/lib/api/generated/model";
import { formatDate } from "@/lib/format/date";
import { formatRupiah } from "@/lib/format/rupiah";

import type {
  ChooseAction,
  ChooseProblem,
  CommitButtonView,
  CountRowView,
  FilterChipView,
  ImportRowView,
  RowChipTone,
  RowFilter,
  RowMessageView,
  StepNumber,
  SummaryTileView,
} from "./type";

// The API's body limit for an import (importTenants: at most 1 MiB). A bigger file is refused in
// the browser, before any request.
export const MAX_FILE_BYTES = 1_048_576;

export const FILE_ACCEPT = ".csv,text/csv";

// The name the browser saves the template under; the API's Content-Disposition says the same.
export const TEMPLATE_FILE_NAME = "template-impor-penyewa.csv";

export const NO_ACCESS_TEXT = "Anda tidak punya akses untuk mengimpor penyewa.";
export const TOO_LARGE_MESSAGE = "Berkas terlalu besar. Paling besar 1 MB.";
export const UNREADABLE_TITLE = "Berkas tidak bisa dibaca";
export const CHANGED_MESSAGE =
  "Ada baris yang berubah sejak diperiksa. Periksa lagi, lalu simpan.";
export const NO_FILE_TEXT = "Belum ada berkas dipilih";
export const CHECK_SAVES_NOTHING_TEXT =
  "Memeriksa tidak menyimpan apa pun. Anda bisa melihat hasilnya dulu, lalu memutuskan.";
export const EMPTY_CELL = "—";
export const ROW_LIST_LABEL = "Baris berkas";

// The format card (boards Impor.dc.html and Impor-Phone.dc.html), with the Indonesian column
// names the template writes.
export const TEMPLATE_EXAMPLE_TEXT =
  "Template berisi dua baris contoh. Hapus baris contoh sebelum mengimpor.";
export const REQUIRED_COLUMNS = [
  "properti",
  "kamar",
  "nama_penyewa",
  "no_wa",
  "sewa",
  "tanggal_mulai",
] as const;
export const OPTIONAL_COLUMNS = [
  "jenis_properti",
  "jaminan",
  "mulai_tagih",
] as const;
export const BILL_FROM_NOTE =
  "mulai_tagih: bulan pertama yang ditagih. Kosongkan untuk menagih sejak tanggal mulai sewa.";
export const OLD_DUE_DAY_NOTE =
  "Kolom jatuh_tempo dari template lama tidak dipakai lagi.";
export const LIMITS_TEXT =
  "Paling banyak 2.000 baris dan 1 MB. Tanggal ditulis 2026-10-01 atau 01/10/2026.";

export const STEPS: readonly StepNumber[] = [1, 2, 3];

export const STEP_LABELS: Record<StepNumber, string> = {
  1: "Pilih berkas",
  2: "Periksa",
  3: "Selesai",
};

export function stepProgressText(step: StepNumber): string {
  return `Langkah ${step} dari 3: ${STEP_LABELS[step]}`;
}

// The page's title and subtitle per step, as on the boards.
export function stepHeading(
  step: StepNumber,
  fileName: string,
): { title: string; subtitle: string } {
  if (step === 2) {
    return {
      title: "Periksa berkas impor",
      subtitle: `${fileName} · belum ada yang disimpan.`,
    };
  }
  if (step === 3) {
    return {
      title: "Impor penyewa",
      subtitle: "Penyewa dan sewa dari berkas sudah tersimpan.",
    };
  }
  return {
    title: "Impor penyewa",
    subtitle: "Masukkan banyak penyewa sekaligus dari spreadsheet.",
  };
}

// The template is a plain link, not a client call: the API client only takes JSON answers
// (src/lib/api/client.ts, "Non-JSON downloads (the CSV template) are links"). A link saves the
// bytes as the API sends them, byte order mark included, which Excel needs.
export function templateDownloadHref(orgId: string): string {
  return `${API_BASE_PATH}/orgs/${encodeURIComponent(orgId)}/imports/tenants/template`;
}

export function isTooLarge(sizeBytes: number): boolean {
  return sizeBytes > MAX_FILE_BYTES;
}

// "38 KB", rounded up so a small file never reads "0 KB".
export function fileSizeText(sizeBytes: number): string {
  const kilobytes = Math.max(1, Math.ceil(sizeBytes / 1024));
  return `${kilobytes.toLocaleString("id-ID")} KB`;
}

// What a failed check means for step 1, by the API's code (clients branch on code, not on the
// HTTP status). invalid_input on the field "file" is a file the API cannot read (missing columns,
// no rows, too many); forbidden is a denial and too_large a file the API found too large, and
// both need another file. Anything else (no connection, a server error, a refused origin) is not
// about the file, so the same file may be tried again.
export function checkProblem(error: unknown): ChooseProblem {
  if (!(error instanceof ApiError)) {
    return {
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
      chooseOther: false,
    };
  }
  if (error.code === "invalid_input" && error.field === "file") {
    return { kind: "unreadable", message: error.message };
  }
  return {
    kind: "alert",
    message: error.message,
    chooseOther: error.code === "forbidden" || error.code === "too_large",
  };
}

export function chooseAction(
  hasFile: boolean,
  problem: ChooseProblem | null,
): ChooseAction {
  if (problem !== null) {
    if (problem.kind === "tooLarge" || problem.kind === "unreadable") {
      return "chooseOther";
    }
    if (problem.chooseOther) return "chooseOther";
  }
  return hasFile ? "check" : "checkDisabled";
}

// A chip's surface by tone, from the theme tokens. The board's blue "Sudah ada, tidak diubah" has
// no token: it is outlined in the accent instead, like "Belum mulai" on the tenant list.
export const ROW_CHIP_TONE_CLASS: Record<RowChipTone, string> = {
  ready: "bg-accent text-primary",
  matched: "border border-primary bg-card text-primary",
  error: "bg-destructive/10 text-destructive",
  neutral: "bg-secondary text-foreground",
};

// The canonical CSV columns the API names in a row error, as the template and the screen write
// them. Anything else stays as the API sent it.
const INDONESIAN_COLUMNS: Readonly<Record<string, string>> = {
  property: "properti",
  unit: "kamar",
  tenant_name: "nama_penyewa",
  phone: "no_wa",
  rent: "sewa",
  start_date: "tanggal_mulai",
  deposit: "jaminan",
  property_kind: "jenis_properti",
  bill_from: "mulai_tagih",
  due_day: "jatuh_tempo",
};

export function indonesianColumn(column: string): string {
  return INDONESIAN_COLUMNS[column] ?? column;
}

function rowStatus(row: ImportRow): ImportRowView["status"] {
  if (row.status === "error") {
    return { label: "Perlu diperbaiki", tone: "error" };
  }
  if (row.lease === "matched") {
    return { label: "Sudah ada, tidak diubah", tone: "matched" };
  }
  return { label: "Siap", tone: "ready" };
}

function cell(value: string): string {
  return value === "" ? EMPTY_CELL : value;
}

// The rent shows only when the row's sewa cell has one; an empty cell uses the unit's price.
function rowPlaceText(row: ImportRow): string {
  const parts = [cell(row.unitLabel), cell(row.propertyName)];
  const rent = row.values?.rentAmount;
  if (rent !== undefined) parts.push(formatRupiah(rent));
  return parts.join(" · ");
}

function rowDatesText(row: ImportRow): string | null {
  if (row.values === undefined) return null;
  const { startDate, billedFrom } = row.values;
  const start = `Mulai ${formatDate(startDate)}`;
  return billedFrom === startDate
    ? start
    : `${start}, tagih dari ${formatDate(billedFrom)}`;
}

function createdChips(row: ImportRow): string[] {
  const chips: string[] = [];
  if (row.property === "created") chips.push("Properti baru");
  if (row.unit === "created") chips.push("Unit baru");
  if (row.tenant === "created") chips.push("Penyewa baru");
  if (row.lease === "created") chips.push("Sewa baru");
  return chips;
}

function errorMessage(error: ImportRow["errors"][number]): RowMessageView {
  return {
    label:
      error.column === "" ? null : `Kolom ${indonesianColumn(error.column)}: `,
    text: error.message,
  };
}

export function importRowView(row: ImportRow): ImportRowView {
  return {
    line: row.line,
    lineLabel: `Baris ${row.line}`,
    status: rowStatus(row),
    tenantName: cell(row.tenantName),
    placeText: rowPlaceText(row),
    datesText: rowDatesText(row),
    createdChips: createdChips(row),
    errors: row.errors.map(errorMessage),
    notes: row.notes.map((note) => ({ label: "Catatan: ", text: note })),
  };
}

export function summaryTiles(summary: ImportSummary): SummaryTileView[] {
  return [
    { label: "Baris", value: summary.rows, warning: false },
    {
      label: "Perlu diperbaiki",
      value: summary.rowsWithErrors,
      warning: summary.rowsWithErrors > 0,
    },
    { label: "Sewa baru", value: summary.leasesCreated, warning: false },
    { label: "Sudah ada", value: summary.leasesMatched, warning: false },
  ];
}

// "Juga akan dibuat: 1 properti, 14 unit, 40 penyewa.", leaving out the parts that are zero;
// null when nothing else would be created.
export function alsoCreatedText(summary: ImportSummary): string | null {
  const parts = [
    [summary.propertiesCreated, "properti"],
    [summary.unitsCreated, "unit"],
    [summary.tenantsCreated, "penyewa"],
  ] as const;
  const shown = parts
    .filter(([count]) => count > 0)
    .map(([count, noun]) => `${count} ${noun}`);
  return shown.length === 0 ? null : `Juga akan dibuat: ${shown.join(", ")}.`;
}

// The commit is all or nothing: the API saves only when no row has an error.
export function commitButton(summary: ImportSummary): CommitButtonView {
  const enabled = summary.rowsWithErrors === 0 && summary.rows > 0;
  return {
    label: `Simpan ${summary.rows - summary.rowsWithErrors} baris`,
    enabled,
    hint: enabled
      ? "Semuanya disimpan sekaligus. Kalau satu gagal, tidak ada yang tersimpan."
      : "Simpan baru bisa kalau semua baris benar. Tidak ada yang tersimpan sebagian.",
  };
}

function matchesFilter(row: ImportRow, filter: RowFilter): boolean {
  if (filter === "errors") return row.status === "error";
  if (filter === "notes") return row.notes.length > 0;
  return true;
}

export function filterRows(
  rows: readonly ImportRow[],
  filter: RowFilter,
): ImportRow[] {
  return rows.filter((row) => matchesFilter(row, filter));
}

export function filterCounts(
  rows: readonly ImportRow[],
): Record<RowFilter, number> {
  return {
    all: rows.length,
    errors: filterRows(rows, "errors").length,
    notes: filterRows(rows, "notes").length,
  };
}

const FILTER_NAMES: Record<RowFilter, string> = {
  all: "Semua",
  errors: "Perlu diperbaiki",
  notes: "Ada catatan",
};

// "Semua 48", then "Perlu diperbaiki 3" and "Ada catatan 5" only when there are such rows (board
// Impor-Siap.dc.html leaves out the empty one).
export function filterChips(rows: readonly ImportRow[]): FilterChipView[] {
  const counts = filterCounts(rows);
  return (["all", "errors", "notes"] as const)
    .filter((filter) => filter === "all" || counts[filter] > 0)
    .map((filter) => ({
      filter,
      label: `${FILTER_NAMES[filter]} ${counts[filter]}`,
    }));
}

export function doneTitle(summary: ImportSummary): string {
  return `Impor selesai: ${summary.rows} baris disimpan`;
}

export function doneText(fileName: string, organizationName: string): string {
  return organizationName === ""
    ? `${fileName} sudah tersimpan.`
    : `${fileName} sudah masuk ke ${organizationName}.`;
}

export function createdCounts(summary: ImportSummary): CountRowView[] {
  return [
    { label: "Properti baru", value: summary.propertiesCreated },
    { label: "Unit baru", value: summary.unitsCreated },
    { label: "Penyewa baru", value: summary.tenantsCreated },
    { label: "Sewa baru", value: summary.leasesCreated },
    { label: "Sudah ada, tidak diubah", value: summary.leasesMatched },
  ];
}
