import { describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api/errors";
import type {
  AccountInvoice,
  Capability,
  CheckDepositGap,
  CheckPhoneChanged,
  CheckQrisAmountMismatch,
  CheckQueueItem,
  CheckTenant,
  CheckUploadInfected,
  MatchTransferProofResult,
  PaymentPreviewItem,
  TenantPaymentPreview,
  Upload,
} from "@/lib/api/generated/model";
import { UploadPurpose } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import {
  canCheckItems,
  canMatchProofs,
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
  PHOTO_ERROR_TEXT,
  previewAmount,
  PROOF_POLL_MS,
  proofMatchDefaults,
  proofMatchedToast,
  proofMatchRequestBody,
  proofMatchSchema,
  proofPayerView,
  proofPhotoAlt,
  proofPhotoCaption,
  proofPhotoState,
  proofPreviewView,
  proofReady,
  PURPOSE_LABEL,
  qrisDifferenceText,
  qrisMismatchSheetView,
  readCheckKind,
  REASON_EMPTY_MESSAGE,
  rejectFormSchema,
  shortName,
  tenantLine,
  uploadInfectedSheetView,
  uploadRefetchMs,
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

  it("matches a proof only with payment:record as well", () => {
    expect(
      canMatchProofs(
        ability([
          { action: "payment:check", access: "allow" },
          { action: "payment:record", access: "allow" },
        ]),
      ),
    ).toBe(true);
    // "conditional" shows it: the API decides per tenant.
    expect(
      canMatchProofs(
        ability([
          { action: "payment:check", access: "allow" },
          { action: "payment:record", access: "conditional" },
        ]),
      ),
    ).toBe(true);
    expect(
      canMatchProofs(
        ability([
          { action: "payment:check", access: "allow" },
          { action: "payment:record", access: "deny" },
        ]),
      ),
    ).toBe(false);
    expect(
      canMatchProofs(
        ability([
          { action: "cash:read", access: "allow" },
          { action: "payment:record", access: "allow" },
        ]),
      ),
    ).toBe(false);
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
  it("shows a transfer proof, openable, ready or still scanned", () => {
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
      opens: true,
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
    [409, "upload_not_ready", "upload_not_ready"],
    [409, "upload_refused", "upload_refused"],
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

const TODAY = "2026-11-15";

function uploaded(overrides: Partial<Upload> = {}): Upload {
  return {
    id: "u-9",
    purpose: "transfer_proof",
    status: "clean",
    size: 120_000,
    createdAt: CREATED,
    downloadUrl:
      "http://localhost:9100/cashflow-dev/files/u-9?X-Amz-Signature=x",
    downloadExpiresAt: "2026-11-14T10:07:00Z",
    ...overrides,
  };
}

function previewItem(
  overrides: Partial<PaymentPreviewItem> = {},
): PaymentPreviewItem {
  return {
    invoiceId: "inv-oct",
    kind: "regular",
    unitLabel: "Kamar 11",
    propertyName: "Kos Melati",
    period: "2026-10-01",
    periodEnd: "2026-10-31",
    dueDate: "2026-10-01",
    balance: 1_050_000,
    applied: 1_050_000,
    balanceAfter: 0,
    paidInFull: true,
    ...overrides,
  };
}

function preview(
  overrides: Partial<TenantPaymentPreview> = {},
): TenantPaymentPreview {
  return {
    amount: 1_050_000,
    outstandingBefore: 2_100_000,
    outstandingAfter: 1_050_000,
    items: [previewItem()],
    creditAdded: 0,
    ...overrides,
  };
}

describe("transfer proof form", () => {
  it("opens with no amount, today, no note", () => {
    expect(proofMatchDefaults(TODAY)).toEqual({
      amount: "",
      receivedOn: TODAY,
      note: "",
    });
  });

  it("sends the integer amount for the proof's own tenant", () => {
    const fields = proofMatchSchema(TODAY).parse({
      amount: "1.050.000",
      receivedOn: "2026-11-13",
      note: "  masuk ke BCA usaha  ",
    });
    expect(proofMatchRequestBody(fields, "req-1")).toEqual({
      requestId: "req-1",
      amount: 1_050_000,
      receivedOn: "2026-11-13",
      note: "masuk ke BCA usaha",
    });
    const bare = proofMatchSchema(TODAY).parse({
      amount: "50000",
      receivedOn: TODAY,
      note: " ",
    });
    const body = proofMatchRequestBody(bare, "req-2");
    expect(body).toEqual({
      requestId: "req-2",
      amount: 50_000,
      receivedOn: TODAY,
    });
    expect(body).not.toHaveProperty("tenantId");
  });

  it("asks for an amount above zero and a day not in the future", () => {
    const schema = proofMatchSchema(TODAY);
    expect(
      schema.safeParse({ amount: "", receivedOn: TODAY, note: "" }).success,
    ).toBe(false);
    expect(
      schema.safeParse({ amount: "0", receivedOn: TODAY, note: "" }).success,
    ).toBe(false);
    expect(
      schema.safeParse({ amount: "1000", receivedOn: "2026-11-16", note: "" })
        .success,
    ).toBe(false);
    expect(
      schema.safeParse({ amount: "1000", receivedOn: "", note: "" }).success,
    ).toBe(false);
    expect(
      schema.safeParse({
        amount: "1000",
        receivedOn: TODAY,
        note: "a".repeat(501),
      }).success,
    ).toBe(false);
  });

  it("previews only an amount the API would take", () => {
    expect(previewAmount("1.050.000")).toBe(1_050_000);
    expect(previewAmount("")).toBeNull();
    expect(previewAmount("0")).toBeNull();
    expect(previewAmount("1.000.000.000.001")).toBeNull();
  });
});

describe("transfer proof photo", () => {
  it("shows the clean photo at its link once getUpload answers", () => {
    expect(proofPhotoState(uploaded(), "clean", null)).toEqual({
      kind: "clean",
      url: "http://localhost:9100/cashflow-dev/files/u-9?X-Amz-Signature=x",
    });
    expect(proofPhotoState(undefined, "clean", null)).toEqual({
      kind: "loading",
    });
    expect(
      proofPhotoState(uploaded({ downloadUrl: undefined }), "clean", null),
    ).toEqual({ kind: "error", message: PHOTO_ERROR_TEXT });
  });

  it("follows getUpload's status over the queue's", () => {
    expect(proofPhotoState(undefined, "pending", null)).toEqual({
      kind: "pending",
    });
    expect(
      proofPhotoState(
        uploaded({ status: "pending", downloadUrl: undefined }),
        "clean",
        null,
      ),
    ).toEqual({ kind: "pending" });
    expect(
      proofPhotoState(uploaded({ status: "infected" }), "pending", null),
    ).toEqual({ kind: "refused" });
    expect(
      proofPhotoState(uploaded({ status: "rejected" }), "pending", null),
    ).toEqual({ kind: "refused" });
    expect(proofReady(undefined, "pending")).toBe(false);
    expect(proofReady(uploaded(), "pending")).toBe(true);
    expect(proofReady(undefined, "clean")).toBe(true);
  });

  it("shows the API's message when the photo cannot be read", () => {
    expect(proofPhotoState(undefined, "clean", "Tidak boleh.")).toEqual({
      kind: "error",
      message: "Tidak boleh.",
    });
    // A failed refetch keeps the photo it had.
    expect(proofPhotoState(uploaded(), "clean", "Tidak boleh.").kind).toBe(
      "clean",
    );
  });

  it("asks again while scanning and before the link expires", () => {
    const now = Date.parse("2026-11-14T10:02:00Z");
    expect(uploadRefetchMs(undefined, now)).toBe(false);
    expect(
      uploadRefetchMs(
        uploaded({ status: "pending", downloadUrl: undefined }),
        now,
      ),
    ).toBe(PROOF_POLL_MS);
    // Five minutes left: 15 seconds before it ends.
    expect(uploadRefetchMs(uploaded(), now)).toBe(5 * 60_000 - 15_000);
    // Almost expired: no faster than the scan poll.
    expect(
      uploadRefetchMs(
        uploaded({ downloadExpiresAt: "2026-11-14T10:02:05Z" }),
        now,
      ),
    ).toBe(PROOF_POLL_MS);
    expect(uploadRefetchMs(uploaded({ status: "infected" }), now)).toBe(false);
    expect(
      uploadRefetchMs(uploaded({ downloadExpiresAt: undefined }), now),
    ).toBe(false);
  });

  it("says when it was sent and from which invoice's link", () => {
    const invoice = {
      id: "inv-nov",
      kind: "regular",
      period: "2026-11-01",
    } as AccountInvoice;
    expect(proofPhotoCaption(CREATED, "inv-nov", [invoice])).toEqual({
      sentAt: "Sab 14 Nov, 17.02",
      link: "November 2026",
    });
    expect(
      proofPhotoCaption(CREATED, "inv-nov", [{ ...invoice, kind: "final" }])
        .link,
    ).toBe("Tagihan akhir November 2026");
    // The account is not loaded, or does not list the invoice: no month.
    expect(proofPhotoCaption(CREATED, "inv-nov", undefined).link).toBeNull();
    expect(proofPhotoCaption(CREATED, "other", [invoice]).link).toBeNull();
    expect(proofPhotoAlt(WATI)).toBe(
      "Foto bukti transfer dari Bu Wati Handayani",
    );
  });
});

describe("transfer proof payer and preview", () => {
  it("names the tenant, the unit and what they owe", () => {
    expect(proofPayerView(WATI, 1_050_000)).toEqual({
      name: "Bu Wati Handayani",
      detail: "Kamar 11, Kos Melati · sisa Rp1.050.000",
    });
    expect(proofPayerView(WATI, undefined).detail).toBe("Kamar 11, Kos Melati");
    expect(proofPayerView(NO_UNIT, 0).detail).toBe("sisa Rp0");
    expect(proofPayerView(NO_UNIT, undefined).detail).toBeNull();
  });

  it("lists what the API says the amount pays", () => {
    const view = proofPreviewView(
      preview({
        amount: 1_300_000,
        items: [
          previewItem(),
          previewItem({
            invoiceId: "inv-nov",
            period: "2026-11-01",
            balance: 1_050_000,
            applied: 250_000,
            balanceAfter: 800_000,
            paidInFull: false,
          }),
        ],
      }),
    );
    expect(view).toEqual({
      amount: 1_300_000,
      lines: [
        { id: "inv-oct", lead: "Tagihan Oktober 2026 jadi", result: "Lunas" },
        {
          id: "inv-nov",
          lead: "Tagihan November 2026:",
          result: "Sebagian, sisa Rp800.000",
        },
      ],
      credit: null,
    });
  });

  it("names the credit, and the unit when the amount reaches several", () => {
    const view = proofPreviewView(
      preview({
        amount: 2_200_000,
        items: [
          previewItem(),
          previewItem({
            invoiceId: "inv-b",
            unitLabel: "Rumah B",
            propertyName: "Kontrakan Mawar",
          }),
        ],
        creditAdded: 100_000,
      }),
    );
    expect(view.lines.map((line) => line.lead)).toEqual([
      "Tagihan Oktober 2026 · Kamar 11 jadi",
      "Tagihan Oktober 2026 · Rumah B jadi",
    ]);
    expect(view.credit).toBe("Kelebihan Rp100.000 jadi saldo");
    // Nothing owed: everything goes to credit.
    expect(
      proofPreviewView(preview({ items: [], creditAdded: 50_000 })),
    ).toMatchObject({ lines: [], credit: "Kelebihan Rp50.000 jadi saldo" });
  });

  it("toasts the amount the API recorded", () => {
    const result = {
      payment: { payment: { amount: 1_050_000 } },
    } as MatchTransferProofResult;
    expect(proofMatchedToast(result, "Bu Wati Handayani")).toEqual({
      tone: "create",
      title: "Pembayaran disimpan",
      description: "Rp1.050.000 dari Bu Wati Handayani",
    });
  });
});
