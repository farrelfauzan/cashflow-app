import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  Capability,
  CheckDepositGap,
  CheckPhoneChanged,
  CheckQrisAmountMismatch,
  CheckQueueItem,
  CheckTenant,
  CheckUploadInfected,
} from "@/lib/api/generated/model";
import { UploadPurpose } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  canCheckItems,
  canReadCheckQueue,
  checkActionErrorKind,
  checkQueueLinkLabel,
  checkRowView,
  checkRowViews,
  depositGapSheetView,
  dismissFormSchema,
  dismissRequestBody,
  filterOptions,
  gapText,
  isForbidden,
  KIND_ICON_TONE,
  KINDS,
  NOTE_EMPTY_MESSAGE,
  NOTE_TOO_LONG_MESSAGE,
  phoneAppliedToast,
  phoneChangeSheetView,
  PURPOSE_LABEL,
  qrisDifferenceText,
  qrisMismatchSheetView,
  readCheckKind,
  REASON_EMPTY_MESSAGE,
  rejectFormSchema,
  shortName,
  tenantLine,
  uploadInfectedSheetView,
  uploadTitle,
} from "./utils";

// 2026-11-14 17:02 in Jakarta (a Saturday).
const CREATED = "2026-11-14T10:02:00Z";

const WATI: CheckTenant = {
  id: "t-1",
  name: "Bu Wati Handayani",
  units: [
    { unitLabel: "Kamar 11", propertyName: "Kos Melati", active: true },
    { unitLabel: "Kamar 3", propertyName: "Kos Melati", active: false },
  ],
};

const NO_UNIT: CheckTenant = { id: "t-2", name: "Pak Joko Susilo", units: [] };

const DIMAS = { staffUserId: "s-1", name: "Mas Dimas Saputra" };
const BUDI = { staffUserId: "s-2", name: "Pak Budi Hartono" };

function gap(overrides: Partial<CheckDepositGap> = {}): CheckDepositGap {
  return {
    depositor: DIMAS,
    expectedAmount: 2_500_000,
    receivedAmount: 2_450_000,
    gap: -50_000,
    paymentCount: 3,
    confirmedBy: BUDI,
    confirmedAt: CREATED,
    ...overrides,
  };
}

function upload(
  overrides: Partial<CheckUploadInfected> = {},
): CheckUploadInfected {
  return {
    uploadId: "u-1",
    purpose: "cash_receipt",
    tenant: { id: "t-3", name: "Bu Siti Aminah", units: [] },
    uploadedBy: DIMAS,
    scannedAt: "2026-11-14T04:07:00Z",
    ...overrides,
  };
}

function qris(
  overrides: Partial<CheckQrisAmountMismatch> = {},
): CheckQrisAmountMismatch {
  return {
    attemptId: "a-1",
    tenant: {
      id: "t-4",
      name: "Pak Joko Susilo",
      units: [
        { unitLabel: "Ruko 2", propertyName: "Ruko Sudirman", active: true },
      ],
    },
    transactionId: "AGP-123",
    qrAmount: 1_000_000,
    recordedAmount: 1_000_000,
    reportedAmount: 950_000,
    ...overrides,
  };
}

function phone(overrides: Partial<CheckPhoneChanged> = {}): CheckPhoneChanged {
  return {
    tenant: WATI,
    currentPhone: "+6281234567890",
    newPhone: "+6281298765432",
    recordedBy: DIMAS,
    note: "Nomor lama hilang",
    ...overrides,
  };
}

function item(overrides: Partial<CheckQueueItem>): CheckQueueItem {
  return { kind: "deposit_gap", id: "i-1", createdAt: CREATED, ...overrides };
}

function ability(capabilities: Capability[]) {
  return createAppAbility(capabilities);
}

describe("who may read and act", () => {
  it("lets an org-wide payment checker act and read", () => {
    const a = ability([{ action: "payment:check", access: "allow" }]);
    expect(canCheckItems(a)).toBe(true);
    expect(canReadCheckQueue(a)).toBe(true);
  });

  it("lets the Supervisor (cash:read everywhere) read only", () => {
    const a = ability([
      { action: "cash:read", access: "allow" },
      { action: "payment:check", access: "deny" },
    ]);
    expect(canCheckItems(a)).toBe(false);
    expect(canReadCheckQueue(a)).toBe(true);
  });

  it("gives an admin limited to some properties neither", () => {
    const a = ability([
      { action: "payment:check", access: "conditional" },
      { action: "cash:read", access: "conditional" },
    ]);
    expect(canCheckItems(a)).toBe(false);
    expect(canReadCheckQueue(a)).toBe(false);
  });

  it("knows a 403", () => {
    expect(
      isForbidden(
        new ApiError({ status: 403, code: "forbidden", message: "x" }),
      ),
    ).toBe(true);
    expect(isForbidden(new Error("x"))).toBe(false);
  });

  it("names Penagihan's link with the number waiting", () => {
    expect(checkQueueLinkLabel(6)).toBe("Antrean cek, 6 menunggu");
    expect(checkQueueLinkLabel(0)).toBe("Antrean cek");
  });
});

describe("filters", () => {
  it("reads a known kind and ignores anything else", () => {
    expect(readCheckKind(new URLSearchParams("kind=deposit_gap"))).toBe(
      "deposit_gap",
    );
    expect(readCheckKind(new URLSearchParams("kind=nope"))).toBeUndefined();
    expect(readCheckKind(new URLSearchParams(""))).toBeUndefined();
  });

  it("lists Semua with the waiting total, then every kind with its count", () => {
    const options = filterOptions(
      {
        transfer_proof: 2,
        deposit_gap: 1,
        upload_infected: 1,
        qris_amount_mismatch: 1,
        phone_changed: 1,
      },
      6,
    );
    expect(options.map((o) => `${o.label} ${o.count}`)).toEqual([
      "Semua 6",
      "Bukti transfer 2",
      "Selisih setoran 1",
      "File ditolak 1",
      "QRIS tidak cocok 1",
      "Nomor baru 1",
    ]);
    expect(options[0]?.kind).toBeNull();
  });

  it("has no counts before the API answered", () => {
    expect(
      filterOptions(undefined, undefined).every((o) => o.count === undefined),
    ).toBe(true);
  });
});

describe("names and lines", () => {
  it.each([
    ["Mas Dimas Saputra", "Mas Dimas"],
    ["Bu Siti Aminah", "Bu Siti"],
    ["Pak Budi", "Pak Budi"],
    ["Oky Firmansyah", "Oky Firmansyah"],
    ["Toko Berkah Jaya", "Toko Berkah Jaya"],
  ])("shortName(%s) is %s", (name, short) => {
    expect(shortName(name)).toBe(short);
  });

  it("names the tenant with its first unit, or alone", () => {
    expect(tenantLine(WATI)).toBe("Bu Wati Handayani · Kamar 11, Kos Melati");
    expect(tenantLine(NO_UNIT)).toBe("Pak Joko Susilo");
  });

  it("says a gap's side", () => {
    expect(gapText(-50_000)).toBe("Kurang Rp50.000");
    expect(gapText(50_000)).toBe("Lebih Rp50.000");
  });

  it("measures a QRIS difference only when a report is known", () => {
    expect(qrisDifferenceText(qris())).toBe("Beda Rp50.000");
    expect(qrisDifferenceText(qris({ reportedAmount: 1_200_000 }))).toBe(
      "Beda Rp200.000",
    );
    expect(qrisDifferenceText(qris({ reportedAmount: undefined }))).toBeNull();
  });

  it("labels every upload purpose", () => {
    for (const purpose of Object.values(UploadPurpose)) {
      expect(PURPOSE_LABEL[purpose]).not.toBe("");
    }
    expect(uploadTitle(upload())).toBe("Foto kwitansi Bu Siti Aminah");
    expect(uploadTitle(upload({ purpose: "transfer_proof" }))).toBe(
      "Bukti transfer Bu Siti Aminah",
    );
    expect(
      uploadTitle(upload({ purpose: "cash_deposit", tenant: undefined })),
    ).toBe("Foto setoran");
  });
});

describe("rows", () => {
  it("shows a transfer proof, not openable, ready or still scanned", () => {
    const proof = {
      tenant: WATI,
      invoiceId: "inv-1",
      uploadId: "u-1",
      uploadStatus: "clean" as const,
    };
    const row = checkRowView(
      item({ kind: "transfer_proof", transferProof: proof }),
    );
    expect(row).toMatchObject({
      kindLabel: "Bukti transfer",
      title: "Bu Wati Handayani · Kamar 11, Kos Melati",
      detail: "Foto bukti dari halaman pembayaran",
      chip: { label: "Siap dicek", tone: "active" },
      arrived: "Masuk Sab 14 Nov, 17.02",
      opens: false,
      iconTone: "accent",
    });
    expect(
      checkRowView(
        item({
          kind: "transfer_proof",
          transferProof: { ...proof, uploadStatus: "pending" },
        }),
      )?.chip,
    ).toEqual({ label: "Foto sedang diperiksa", tone: "neutral" });
  });

  it("shows a deposit gap", () => {
    expect(
      checkRowView(item({ kind: "deposit_gap", depositGap: gap() })),
    ).toMatchObject({
      title: "Mas Dimas Saputra · 3 kwitansi",
      detail: "Total kwitansi Rp2.500.000 · diterima Rp2.450.000",
      chip: { label: "Kurang Rp50.000", tone: "warning" },
      opens: true,
      iconTone: "warning",
    });
    expect(
      checkRowView(
        item({
          kind: "deposit_gap",
          depositGap: gap({ receivedAmount: 2_550_000, gap: 50_000 }),
        }),
      )?.chip.label,
    ).toBe("Lebih Rp50.000");
  });

  it.each([
    [{}, "Foto kwitansi Bu Siti Aminah", "Diunggah Mas Dimas Saputra"],
    [
      { purpose: "transfer_proof" as const, uploadedBy: undefined },
      "Bukti transfer Bu Siti Aminah",
      "Dikirim penyewa",
    ],
    [
      { purpose: "cash_deposit" as const, tenant: undefined },
      "Foto setoran",
      "Diunggah Mas Dimas Saputra",
    ],
  ])("shows an infected upload %#", (overrides, title, detail) => {
    expect(
      checkRowView(
        item({ kind: "upload_infected", uploadInfected: upload(overrides) }),
      ),
    ).toMatchObject({
      title,
      detail,
      chip: { label: "Berbahaya", tone: "warning" },
    });
  });

  it("shows a QRIS mismatch with and without AutoGoPay's report", () => {
    expect(
      checkRowView(
        item({ kind: "qris_amount_mismatch", qrisAmountMismatch: qris() }),
      ),
    ).toMatchObject({
      title: "Pak Joko Susilo · Ruko 2, Ruko Sudirman",
      detail: "QR Rp1.000.000 · dilaporkan Rp950.000",
      chip: { label: "Beda Rp50.000", tone: "warning" },
    });
    expect(
      checkRowView(
        item({
          kind: "qris_amount_mismatch",
          qrisAmountMismatch: qris({ reportedAmount: undefined }),
        }),
      ),
    ).toMatchObject({
      detail: "QR Rp1.000.000 · laporan AutoGoPay tidak diketahui",
      chip: { label: "Perlu dicek", tone: "warning" },
    });
  });

  it("shows a new number", () => {
    expect(
      checkRowView(item({ kind: "phone_changed", phoneChanged: phone() })),
    ).toMatchObject({
      title: "Bu Wati Handayani · Kamar 11, Kos Melati",
      detail: "Dicatat Mas Dimas Saputra saat kunjungan",
      chip: { label: "Perlu diterapkan", tone: "neutral" },
      iconTone: "accent",
    });
  });

  it("leaves out an item sent without its kind's object", () => {
    expect(checkRowView(item({ kind: "phone_changed" }))).toBeNull();
    expect(
      checkRowViews([
        item({ kind: "deposit_gap" }),
        item({ id: "i-2", depositGap: gap() }),
      ]).map((row) => row.id),
    ).toEqual(["i-2"]);
  });

  it("tints the money kinds with the warning", () => {
    expect(KINDS.filter((kind) => KIND_ICON_TONE[kind] === "warning")).toEqual([
      "deposit_gap",
      "upload_infected",
      "qris_amount_mismatch",
    ]);
  });
});

describe("drawers", () => {
  it("builds Selisih setoran", () => {
    const view = depositGapSheetView(gap());
    expect(view.subtitle).toBe(
      "Mas Dimas Saputra · dihitung Pak Budi Hartono, Sab 14 Nov, 17.02",
    );
    expect(view.tiles).toEqual([
      {
        label: "Total kwitansi",
        value: "Rp2.500.000",
        sub: "3 kwitansi",
        tone: "plain",
      },
      {
        label: "Diterima admin",
        value: "Rp2.450.000",
        sub: null,
        tone: "plain",
      },
      {
        label: "Selisih",
        value: "Kurang Rp50.000",
        sub: null,
        tone: "warning",
      },
    ]);
    expect(view.todo).toMatch(/^Tanyakan ke Mas Dimas\. Setelah selisihnya/);
    expect(
      depositGapSheetView(
        gap({ depositor: { staffUserId: "s", name: "Oky Firmansyah" } }),
      ).todo,
    ).toMatch(/^Tanyakan ke Oky Firmansyah\./);
  });

  it("builds File ditolak for every purpose", () => {
    const receipt = uploadInfectedSheetView(upload());
    expect(receipt.subtitle).toBe(
      "Foto kwitansi Bu Siti Aminah · diunggah Mas Dimas Saputra",
    );
    expect(receipt.scannedLine).toBe(
      "Diperiksa Sab 14 Nov, 11.07 · sudah diamankan",
    );
    expect(receipt.todo).toBe(
      "Minta Mas Dimas memotret kwitansi Bu Siti Aminah lagi. Di HP-nya kwitansi ini sudah tampil “Foto ditolak, ambil ulang”.",
    );
    const proof = uploadInfectedSheetView(
      upload({ purpose: "transfer_proof", uploadedBy: undefined }),
    );
    expect(proof.subtitle).toBe(
      "Bukti transfer Bu Siti Aminah · dikirim penyewa",
    );
    expect(proof.todo).toBe(
      "Minta penyewa mengirim foto lagi dari halaman pembayaran.",
    );
    expect(
      uploadInfectedSheetView(upload({ purpose: "cash_deposit" })).todo,
    ).toBe("Minta Mas Dimas memotret uang setoran lagi.");
    expect(
      uploadInfectedSheetView(upload({ scannedAt: undefined })).scannedLine,
    ).toBe("Sudah diamankan");
  });

  it("builds QRIS tidak cocok with a report and the recorded amount", () => {
    const view = qrisMismatchSheetView(qris());
    expect(view.subtitle).toBe("Pak Joko Susilo · Ruko 2, Ruko Sudirman");
    expect(view.tiles.map((t) => [t.label, t.value, t.tone])).toEqual([
      ["Jumlah QR", "Rp1.000.000", "plain"],
      ["Dilaporkan AutoGoPay", "Rp950.000", "warning"],
      ["Tercatat", "Rp1.000.000", "plain"],
    ]);
    expect(view.callout).toBe(
      "Pembayaran sudah tercatat Rp1.000.000 sesuai QR, dan tagihan Pak Joko Susilo sudah dihitung dari jumlah itu.",
    );
    expect(view.tenantLinkText).toBe("Buka Pak Joko Susilo");
    expect(view.tenantId).toBe("t-4");
    expect(view.transactionId).toBe("AGP-123");
  });

  it("builds QRIS tidak cocok without a report or a recorded amount", () => {
    const view = qrisMismatchSheetView(
      qris({ reportedAmount: undefined, recordedAmount: undefined }),
    );
    expect(view.tiles).toEqual([
      { label: "Jumlah QR", value: "Rp1.000.000", sub: null, tone: "plain" },
      {
        label: "Dilaporkan AutoGoPay",
        value: "Tidak diketahui",
        sub: "tercatat oleh pengecekan otomatis",
        tone: "plain",
      },
    ]);
    expect(view.callout).toBe(
      "Pembayaran sudah tercatat sesuai QR, dan tagihan Pak Joko Susilo sudah dihitung dari jumlah itu.",
    );
  });

  it("builds Nomor baru with and without the karyawan's note", () => {
    const view = phoneChangeSheetView(phone(), CREATED);
    expect(view.tiles).toEqual([
      {
        label: "Nomor sekarang",
        value: "0812-3456-7890",
        sub: "di data penyewa",
        tone: "plain",
      },
      {
        label: "Nomor baru",
        value: "0812-9876-5432",
        sub: "dari kunjungan Mas Dimas Saputra",
        tone: "accent",
      },
    ]);
    expect(view.noteCaption).toBe(
      "Catatan Mas Dimas Saputra · Sab 14 Nov, 17.02",
    );
    expect(view.note).toBe("“Nomor lama hilang”");
    expect(view.keepLine).toBe(
      "Nomor Bu Wati Handayani tetap 0812-3456-7890. Item ini hilang dari antrean.",
    );
    expect(phoneAppliedToast(view)).toEqual({
      tone: "create",
      title: "Nomor Bu Wati Handayani diganti",
      description: null,
    });
    const bare = phoneChangeSheetView(phone({ note: "  " }), CREATED);
    expect(bare.noteCaption).toBeNull();
    expect(bare.note).toBeNull();
  });
});

describe("note forms", () => {
  it("trims and sends the note", () => {
    const parsed = dismissFormSchema.parse({ note: "  sudah dicek  " });
    expect(dismissRequestBody(parsed)).toEqual({ note: "sudah dicek" });
  });

  it("asks for a note and for a reason", () => {
    const note = dismissFormSchema.safeParse({ note: "   " });
    expect(note.success).toBe(false);
    expect(note.error?.issues[0]?.message).toBe(NOTE_EMPTY_MESSAGE);
    const reason = rejectFormSchema.safeParse({ note: "" });
    expect(reason.error?.issues[0]?.message).toBe(REASON_EMPTY_MESSAGE);
  });

  it("refuses more than 500 letters after trimming", () => {
    expect(dismissFormSchema.safeParse({ note: "a".repeat(500) }).success).toBe(
      true,
    );
    expect(
      dismissFormSchema.safeParse({ note: ` ${"a".repeat(500)} ` }).success,
    ).toBe(true);
    const long = dismissFormSchema.safeParse({ note: "a".repeat(501) });
    expect(long.error?.issues[0]?.message).toBe(NOTE_TOO_LONG_MESSAGE);
  });
});

describe("action errors", () => {
  it.each([
    [409, "check_item_resolved", "resolved"],
    [409, "phone_taken", "phone_taken"],
    [409, "request_id_reused", "other"],
    [403, "forbidden", "other"],
    [400, "invalid_input", "other"],
  ])("%i %s is %s", (status, code, kind) => {
    expect(
      checkActionErrorKind(new ApiError({ status, code, message: "x" })),
    ).toBe(kind);
  });

  it("treats a non-API error as other", () => {
    expect(checkActionErrorKind(new Error("x"))).toBe("other");
  });
});
