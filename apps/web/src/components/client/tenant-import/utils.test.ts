import { describe, expect, it } from "vitest";

import { ApiError, GENERIC_ERROR_MESSAGE } from "@/lib/api/errors";
import type { ImportRow, ImportSummary } from "@/lib/api/generated/model";

import {
  alsoCreatedText,
  checkProblem,
  COLUMN_NOTES,
  chooseAction,
  commitButton,
  createdCounts,
  doneText,
  doneTitle,
  fileSizeText,
  filterChips,
  filterCounts,
  filterRows,
  importRowView,
  indonesianColumn,
  isTooLarge,
  OPTIONAL_COLUMNS,
  REQUIRED_COLUMNS,
  MAX_FILE_BYTES,
  stepHeading,
  stepProgressText,
  summaryTiles,
  templateDownloadHref,
} from "./utils";

const ORG_ID = "11111111-1111-4111-8111-111111111111";

const row = (over: Partial<ImportRow> = {}): ImportRow => ({
  line: 2,
  status: "ok",
  errors: [],
  notes: [],
  property: "matched",
  unit: "created",
  tenant: "created",
  lease: "created",
  values: {
    startDate: "2026-10-01",
    billedFrom: "2026-10-01",
    termMonths: 1,
    rentAmount: 1_300_000,
    depositAmount: 0,
  },
  propertyName: "Kos Melati",
  unitLabel: "Kamar 11",
  tenantName: "Mas Wahyu Hidayat",
  ...over,
});

const summary = (over: Partial<ImportSummary> = {}): ImportSummary => ({
  rows: 48,
  rowsWithErrors: 3,
  propertiesCreated: 1,
  unitsCreated: 14,
  tenantsCreated: 40,
  leasesCreated: 43,
  leasesMatched: 2,
  ...over,
});

describe("the size check", () => {
  it.each([
    { name: "an empty file", size: 0, want: false },
    { name: "exactly 1 MiB", size: MAX_FILE_BYTES, want: false },
    { name: "one byte over 1 MiB", size: MAX_FILE_BYTES + 1, want: true },
    { name: "2 MB", size: 2_000_000, want: true },
  ])("$name", ({ size, want }) => {
    expect(isTooLarge(size)).toBe(want);
  });

  it("is the API's 1 MiB", () => {
    expect(MAX_FILE_BYTES).toBe(1_048_576);
  });
});

describe("fileSizeText", () => {
  it.each([
    { size: 1, want: "1 KB" },
    { size: 1024, want: "1 KB" },
    { size: 38_000, want: "38 KB" },
    { size: MAX_FILE_BYTES, want: "1.024 KB" },
  ])("$size bytes is $want", ({ size, want }) => {
    expect(fileSizeText(size)).toBe(want);
  });
});

describe("templateDownloadHref", () => {
  it("is the API's template path under /api", () => {
    expect(templateDownloadHref(ORG_ID)).toBe(
      `/api/orgs/${ORG_ID}/imports/tenants/template`,
    );
  });

  it("encodes the organization id so it cannot move the path", () => {
    expect(templateDownloadHref("../auth/me?x=")).toBe(
      "/api/orgs/..%2Fauth%2Fme%3Fx%3D/imports/tenants/template",
    );
  });
});

describe("checkProblem", () => {
  const apiError = (
    status: number,
    code: string,
    field?: string,
    message = "Pesan API.",
  ) => new ApiError({ status, code, message, field });

  it("reads invalid_input on the file as a file the API cannot read, with its message", () => {
    expect(
      checkProblem(
        apiError(400, "invalid_input", "file", "Kolom wajib tidak ada: no_wa."),
      ),
    ).toEqual({ kind: "unreadable", message: "Kolom wajib tidak ada: no_wa." });
  });

  it("does not blame the file for invalid_input on another field", () => {
    expect(checkProblem(apiError(400, "invalid_input", "dryRun"))).toEqual({
      kind: "alert",
      message: "Pesan API.",
      chooseOther: false,
    });
  });

  it.each([
    [403, "forbidden"],
    [413, "too_large"],
  ])("asks for another file after a %i %s", (status, code) => {
    expect(checkProblem(apiError(status, code))).toEqual({
      kind: "alert",
      message: "Pesan API.",
      chooseOther: true,
    });
  });

  it.each([
    [403, "bad_origin"],
    [0, "network_error"],
    [500, "internal"],
    [503, "unavailable"],
  ])("lets a %i %s be tried again with the same file", (status, code) => {
    expect(checkProblem(apiError(status, code))).toEqual({
      kind: "alert",
      message: "Pesan API.",
      chooseOther: false,
    });
  });

  it("never shows another error's text", () => {
    expect(checkProblem(new Error("raw text"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
      chooseOther: false,
    });
  });
});

describe("chooseAction", () => {
  it.each<{
    name: string;
    hasFile: boolean;
    problem: Parameters<typeof chooseAction>[1];
    want: ReturnType<typeof chooseAction>;
  }>([
    { name: "no file", hasFile: false, problem: null, want: "checkDisabled" },
    { name: "a file", hasFile: true, problem: null, want: "check" },
    {
      name: "a file too large",
      hasFile: true,
      problem: { kind: "tooLarge" },
      want: "chooseOther",
    },
    {
      name: "an unreadable file",
      hasFile: true,
      problem: { kind: "unreadable", message: "x" },
      want: "chooseOther",
    },
    {
      name: "a denial",
      hasFile: true,
      problem: { kind: "alert", message: "x", chooseOther: true },
      want: "chooseOther",
    },
    {
      name: "no connection",
      hasFile: true,
      problem: { kind: "alert", message: "x", chooseOther: false },
      want: "check",
    },
  ])("$name", ({ hasFile, problem, want }) => {
    expect(chooseAction(hasFile, problem)).toBe(want);
  });
});

describe("format card", () => {
  // The template's header (services/api/internal/tenantimport/template.go templateColumns).
  const TEMPLATE = [
    "properti",
    "jenis_properti",
    "kamar",
    "nama_penyewa",
    "no_wa",
    "sewa",
    "jangka_bulan",
    "tanggal_mulai",
    "mulai_tagih",
    "jaminan",
  ];

  it("lists every template column once, each group in the template's order", () => {
    const inTemplateOrder = (columns: readonly string[]) =>
      TEMPLATE.filter((column) => columns.includes(column));
    expect([...REQUIRED_COLUMNS]).toEqual(inTemplateOrder(REQUIRED_COLUMNS));
    expect([...OPTIONAL_COLUMNS]).toEqual(inTemplateOrder(OPTIONAL_COLUMNS));
    expect([...REQUIRED_COLUMNS, ...OPTIONAL_COLUMNS].sort()).toEqual(
      [...TEMPLATE].sort(),
    );
  });

  it("explains sewa, jangka_bulan and mulai_tagih in that order", () => {
    expect(COLUMN_NOTES).toEqual([
      "sewa: harga satu periode. Boleh dikosongkan kalau unit sudah punya harga untuk jangka itu.",
      "jangka_bulan: 1, 3, 6 atau 12. Kosongkan untuk 1 bulan, atau satu-satunya jangka yang ditawarkan unit.",
      "mulai_tagih: awal periode pertama yang ditagih. Kosongkan untuk menagih sejak tanggal mulai sewa.",
    ]);
  });
});

describe("indonesianColumn", () => {
  it.each([
    ["property", "properti"],
    ["unit", "kamar"],
    ["tenant_name", "nama_penyewa"],
    ["phone", "no_wa"],
    ["rent", "sewa"],
    ["term", "jangka_bulan"],
    ["start_date", "tanggal_mulai"],
    ["deposit", "jaminan"],
    ["property_kind", "jenis_properti"],
    ["bill_from", "mulai_tagih"],
    ["due_day", "jatuh_tempo"],
    ["something_else", "something_else"],
  ])("%s is %s", (column, want) => {
    expect(indonesianColumn(column)).toBe(want);
  });
});

describe("importRowView", () => {
  it("shows a ready row with what it creates", () => {
    expect(importRowView(row())).toEqual({
      line: 2,
      lineLabel: "Baris 2",
      status: { label: "Siap", tone: "ready" },
      tenantName: "Mas Wahyu Hidayat",
      placeText: "Kamar 11 · Kos Melati · Per bulan · Rp1.300.000",
      datesText: "Mulai 1 Okt 2026",
      createdChips: ["Unit baru", "Penyewa baru", "Sewa baru"],
      errors: [],
      notes: [],
    });
  });

  it("leaves the rent out when the sewa cell is empty", () => {
    const view = importRowView(
      row({
        values: {
          startDate: "2026-10-01",
          billedFrom: "2026-10-01",
          termMonths: 12,
          depositAmount: 0,
        },
      }),
    );
    expect(view.placeText).toBe("Kamar 11 · Kos Melati · Per tahun");
  });

  it.each([
    {
      termMonths: 3,
      want: "Kamar 11 · Kos Melati · Per 3 bulan · Rp3.600.000",
    },
    {
      termMonths: 6,
      want: "Kamar 11 · Kos Melati · Per 6 bulan · Rp3.600.000",
    },
    { termMonths: 12, want: "Kamar 11 · Kos Melati · Per tahun · Rp3.600.000" },
  ] as const)(
    "names the row's term of $termMonths months",
    ({ termMonths, want }) => {
      const view = importRowView(
        row({
          values: {
            startDate: "2026-10-01",
            billedFrom: "2026-10-01",
            termMonths,
            rentAmount: 3_600_000,
            depositAmount: 0,
          },
        }),
      );
      expect(view.placeText).toBe(want);
    },
  );

  it("adds the billing start when it differs from the start date", () => {
    const view = importRowView(
      row({
        values: {
          startDate: "2026-09-15",
          billedFrom: "2026-10-15",
          termMonths: 1,
          rentAmount: 1_300_000,
          depositAmount: 0,
        },
      }),
    );
    expect(view.datesText).toBe("Mulai 15 Sep 2026, tagih dari 15 Okt 2026");
  });

  it("shows a matched lease as already there, with its note", () => {
    const view = importRowView(
      row({
        property: "matched",
        unit: "matched",
        tenant: "matched",
        lease: "matched",
        notes: ["Sewa ini sudah ada dengan isian lain."],
      }),
    );
    expect(view.status).toEqual({
      label: "Sudah ada, tidak diubah",
      tone: "matched",
    });
    expect(view.createdChips).toEqual([]);
    expect(view.notes).toEqual([
      { label: "Catatan: ", text: "Sewa ini sudah ada dengan isian lain." },
    ]);
  });

  it("lists every chip when a row creates everything", () => {
    expect(importRowView(row({ property: "created" })).createdChips).toEqual([
      "Properti baru",
      "Unit baru",
      "Penyewa baru",
      "Sewa baru",
    ]);
  });

  it("shows a row with errors in Indonesian columns, without unread terms", () => {
    const view = importRowView(
      row({
        status: "error",
        property: undefined,
        unit: undefined,
        tenant: undefined,
        lease: undefined,
        values: undefined,
        errors: [
          { column: "phone", message: "Nomor WhatsApp tidak valid." },
          { column: "start_date", message: "Tanggal mulai harus seperti ..." },
          { column: "", message: "Baris ini tidak bisa diperiksa." },
        ],
      }),
    );
    expect(view.status).toEqual({ label: "Perlu diperbaiki", tone: "error" });
    expect(view.placeText).toBe("Kamar 11 · Kos Melati");
    expect(view.datesText).toBeNull();
    expect(view.createdChips).toEqual([]);
    expect(view.errors).toEqual([
      { label: "Kolom no_wa: ", text: "Nomor WhatsApp tidak valid." },
      {
        label: "Kolom tanggal_mulai: ",
        text: "Tanggal mulai harus seperti ...",
      },
      { label: null, text: "Baris ini tidak bisa diperiksa." },
    ]);
  });

  it("shows a dash for empty cells", () => {
    const view = importRowView(
      row({
        tenantName: "",
        unitLabel: "",
        propertyName: "",
        values: undefined,
      }),
    );
    expect(view.tenantName).toBe("—");
    expect(view.placeText).toBe("— · —");
  });
});

describe("summary texts", () => {
  it("shows the four tiles, with the errors in the warning style", () => {
    expect(summaryTiles(summary())).toEqual([
      { label: "Baris", value: 48, warning: false },
      { label: "Perlu diperbaiki", value: 3, warning: true },
      { label: "Sewa baru", value: 43, warning: false },
      { label: "Sudah ada", value: 2, warning: false },
    ]);
  });

  it("shows no errors in the plain style", () => {
    expect(summaryTiles(summary({ rowsWithErrors: 0 }))[1]?.warning).toBe(
      false,
    );
  });

  it.each([
    {
      name: "all three",
      over: {},
      want: "Juga akan dibuat: 1 properti, 14 unit, 40 penyewa.",
    },
    {
      name: "no property",
      over: { propertiesCreated: 0 },
      want: "Juga akan dibuat: 14 unit, 40 penyewa.",
    },
    {
      name: "only tenants",
      over: { propertiesCreated: 0, unitsCreated: 0 },
      want: "Juga akan dibuat: 40 penyewa.",
    },
    {
      name: "nothing",
      over: { propertiesCreated: 0, unitsCreated: 0, tenantsCreated: 0 },
      want: null,
    },
  ])("also created: $name", ({ over, want }) => {
    expect(alsoCreatedText(summary(over))).toBe(want);
  });

  it("titles and describes the done step", () => {
    expect(doneTitle(summary({ rows: 2 }))).toBe(
      "Impor selesai: 2 baris disimpan",
    );
    expect(doneText("penyewa.csv", "Griya Sejahtera")).toBe(
      "penyewa.csv sudah masuk ke Griya Sejahtera.",
    );
    expect(doneText("penyewa.csv", "")).toBe("penyewa.csv sudah tersimpan.");
  });

  it("counts what was created", () => {
    expect(createdCounts(summary())).toEqual([
      { label: "Properti baru", value: 1 },
      { label: "Unit baru", value: 14 },
      { label: "Penyewa baru", value: 40 },
      { label: "Sewa baru", value: 43 },
      { label: "Sudah ada, tidak diubah", value: 2 },
    ]);
  });

  it("titles each step", () => {
    expect(stepHeading(1, "")).toEqual({
      title: "Impor penyewa",
      subtitle: "Masukkan banyak penyewa sekaligus dari spreadsheet.",
    });
    expect(stepHeading(2, "penyewa.csv")).toEqual({
      title: "Periksa berkas impor",
      subtitle: "penyewa.csv · belum ada yang disimpan.",
    });
    expect(stepHeading(3, "penyewa.csv").title).toBe("Impor penyewa");
    expect(stepProgressText(2)).toBe("Langkah 2 dari 3: Periksa");
  });
});

describe("filters", () => {
  const rows = [
    row({ line: 2 }),
    row({
      line: 3,
      status: "error",
      errors: [{ column: "phone", message: "x" }],
    }),
    row({ line: 4, notes: ["catatan"] }),
    row({
      line: 5,
      status: "error",
      errors: [{ column: "unit", message: "y" }],
      notes: ["catatan"],
    }),
  ];

  it("counts each filter", () => {
    expect(filterCounts(rows)).toEqual({ all: 4, errors: 2, notes: 2 });
  });

  it.each([
    { filter: "all" as const, want: [2, 3, 4, 5] },
    { filter: "errors" as const, want: [3, 5] },
    { filter: "notes" as const, want: [4, 5] },
  ])("filters $filter", ({ filter, want }) => {
    expect(filterRows(rows, filter).map((r) => r.line)).toEqual(want);
  });

  it("labels the chips with their counts", () => {
    expect(filterChips(rows)).toEqual([
      { filter: "all", label: "Semua 4" },
      { filter: "errors", label: "Perlu diperbaiki 2" },
      { filter: "notes", label: "Ada catatan 2" },
    ]);
  });

  it("leaves out a chip with no rows", () => {
    expect(filterChips([row()])).toEqual([{ filter: "all", label: "Semua 1" }]);
  });
});

describe("commitButton", () => {
  it("is disabled while a row has an error, counting the good rows", () => {
    expect(commitButton(summary())).toEqual({
      label: "Simpan 45 baris",
      enabled: false,
      hint: "Simpan baru bisa kalau semua baris benar. Tidak ada yang tersimpan sebagian.",
    });
  });

  it("saves every row when none has an error", () => {
    expect(commitButton(summary({ rows: 2, rowsWithErrors: 0 }))).toEqual({
      label: "Simpan 2 baris",
      enabled: true,
      hint: "Semuanya disimpan sekaligus. Kalau satu gagal, tidak ada yang tersimpan.",
    });
  });

  it("is disabled with no rows", () => {
    expect(commitButton(summary({ rows: 0, rowsWithErrors: 0 })).enabled).toBe(
      false,
    );
  });
});
