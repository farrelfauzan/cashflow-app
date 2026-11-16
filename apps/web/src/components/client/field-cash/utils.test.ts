import { describe, expect, it, vi } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type {
  CapabilityAccess,
  CashPaymentResult,
  UploadTicket,
} from "@/lib/api/generated/model";
import { StorageUploadError } from "@/lib/api/storage-upload";
import { createAppAbility } from "@/lib/capabilities";

import type {
  ChosenPhoto,
  ReceiptPhotoState,
  PhotoUploadSteps,
  ReceiveCashTenant,
} from "./type";
import {
  ALL_CREDIT_TEXT,
  PHOTO_NETWORK_TEXT,
  PHOTO_SENDING_TEXT,
  PHOTO_TOO_LARGE_TEXT,
  SAVE_TEXT,
  SAVING_TEXT,
  UNKNOWN_SAVE_TEXT,
  WAITING_PHOTO_TEXT,
  canRecordCash,
  canSave,
  cashFormSchema,
  classifyUploadFailure,
  cashPaymentBody,
  cashRecordedView,
  notRecordedLine,
  owedLine,
  photoAnnouncement,
  photoLine,
  receiptLine,
  tenantUploadInput,
  recordErrorView,
  saveLabel,
  sheetSubtitle,
  uploadFailureView,
  uploadingLine,
  uploadTenantPhoto,
} from "./utils";

const PHOTO: ChosenPhoto = {
  file: new File([new Uint8Array(10)], "kwitansi.jpg", { type: "image/jpeg" }),
  previewUrl: "blob:preview",
};

const SENT: ReceiptPhotoState = {
  kind: "sent",
  photo: PHOTO,
  uploadId: "8f1c2e9a-0b7d-4f43-9a51-2c6a1d0e5b77",
};

function tenant(overrides: Partial<ReceiveCashTenant>): ReceiveCashTenant {
  return {
    tenantId: "b0d4c7a2-5e61-4a8f-9c3d-7e2f1a6b8c90",
    tenantName: "Bu Wati Handayani",
    reason: "overdue",
    daysOverdue: 7,
    oldestUnpaidDueDate: "2026-11-09",
    outstanding: 1_050_000,
    ...overrides,
  };
}

function apiError(
  status: number,
  code: string,
  message: string,
  field?: string,
): ApiError {
  return new ApiError({ status, code, message, field });
}

describe("cashFormSchema", () => {
  it.each([
    ["", "Isi jumlah yang diterima."],
    ["0", "Jumlah harus lebih dari Rp0."],
    ["1.000.000.000.001", "Jumlah paling banyak Rp1.000.000.000.000."],
  ])("refuses %j", (amount, message) => {
    const parsed = cashFormSchema.safeParse({ amount });
    expect(parsed.success).toBe(false);
    expect(parsed.error?.issues[0]?.message).toBe(message);
  });

  it("turns the grouped digits into integer rupiah", () => {
    expect(cashFormSchema.parse({ amount: "1.000.000" })).toEqual({
      amount: 1_000_000,
    });
  });
});

describe("canRecordCash", () => {
  it.each<[CapabilityAccess, boolean]>([
    ["allow", true],
    ["conditional", true],
    ["deny", false],
  ])("cash:record %s gives %s", (access, shown) => {
    const ability = createAppAbility([{ action: "cash:record", access }]);
    expect(canRecordCash(ability)).toBe(shown);
  });

  it("is hidden when the API does not list cash:record", () => {
    const ability = createAppAbility([
      { action: "payment:record", access: "allow" },
    ]);
    expect(canRecordCash(ability)).toBe(false);
  });
});

describe("sheetSubtitle", () => {
  it("joins the name and the place", () => {
    expect(sheetSubtitle("Bu Wati Handayani", "Kos Melati 11")).toBe(
      "Bu Wati Handayani · Kos Melati 11",
    );
  });

  it("is the name alone without a place", () => {
    expect(sheetSubtitle("Bu Wati Handayani", " ")).toBe("Bu Wati Handayani");
  });
});

describe("owedLine", () => {
  it("names the due date and the days late", () => {
    expect(owedLine(tenant({}))).toBe("Jatuh tempo 9 Nov 2026, telat 7 hari");
  });

  it("says only the days late without a due date", () => {
    expect(owedLine(tenant({ oldestUnpaidDueDate: undefined }))).toBe(
      "Telat 7 hari",
    );
  });

  it.each<[Partial<ReceiveCashTenant>, string | null]>([
    [{ reason: "due_today", daysOverdue: undefined }, "Jatuh tempo hari ini"],
    [
      { reason: "due_in_days", daysOverdue: undefined, daysUntilDue: 1 },
      "Jatuh tempo besok",
    ],
    [
      { reason: "due_in_days", daysOverdue: undefined, daysUntilDue: 2 },
      "Jatuh tempo lusa",
    ],
    [
      {
        reason: "paid_today",
        daysOverdue: undefined,
        oldestUnpaidDueDate: "2026-12-15",
      },
      "Jatuh tempo 15 Des 2026",
    ],
    [
      {
        reason: "paid_today",
        daysOverdue: undefined,
        oldestUnpaidDueDate: undefined,
        outstanding: 0,
      },
      null,
    ],
  ])("%j gives %j", (overrides, line) => {
    expect(owedLine(tenant(overrides))).toBe(line);
  });

  it("leaves out a due date it cannot read", () => {
    expect(owedLine(tenant({ oldestUnpaidDueDate: "kemarin" }))).toBe(
      "Telat 7 hari",
    );
  });
});

describe("request bodies", () => {
  it("starts a cash_receipt upload for the tenant with the file's type and exact size", () => {
    expect(tenantUploadInput("cash_receipt", "tenant-1", PHOTO.file)).toEqual({
      purpose: "cash_receipt",
      tenantId: "tenant-1",
      contentType: "image/jpeg",
      size: 10,
    });
  });

  it("starts a transfer_proof upload for the tenant the same way", () => {
    expect(tenantUploadInput("transfer_proof", "tenant-1", PHOTO.file)).toEqual(
      {
        purpose: "transfer_proof",
        tenantId: "tenant-1",
        contentType: "image/jpeg",
        size: 10,
      },
    );
  });

  it("records the integer amount with the opening's requestId and the photo", () => {
    expect(cashPaymentBody(1_000_000, "req-1", "upload-1")).toEqual({
      requestId: "req-1",
      amount: 1_000_000,
      photoUploadId: "upload-1",
    });
  });
});

describe("saveLabel and canSave", () => {
  it("names the typed amount once the photo is sent", () => {
    expect(saveLabel(SENT, "1.000.000", false)).toBe("Simpan Rp1.000.000");
    expect(canSave(SENT, false)).toBe(true);
  });

  it("says Simpan alone before an amount is typed, and still lets it say why", () => {
    expect(saveLabel(SENT, "", false)).toBe(SAVE_TEXT);
    expect(canSave(SENT, false)).toBe(true);
  });

  it("waits for the photo while it is sent", () => {
    const uploading: ReceiptPhotoState = {
      kind: "uploading",
      photo: PHOTO,
      percent: 64,
    };
    expect(saveLabel(uploading, "1.000.000", false)).toBe(WAITING_PHOTO_TEXT);
    expect(canSave(uploading, false)).toBe(false);
  });

  it.each<ReceiptPhotoState>([
    { kind: "empty" },
    {
      kind: "failed",
      photo: PHOTO,
      reason: "network",
      message: PHOTO_NETWORK_TEXT,
      resume: { step: "create" },
    },
    { kind: "refused", photo: PHOTO, message: PHOTO_TOO_LARGE_TEXT },
  ])(
    "is disabled without a sent photo ($kind), still naming the amount",
    (photo) => {
      expect(saveLabel(photo, "1.000.000", false)).toBe("Simpan Rp1.000.000");
      expect(canSave(photo, false)).toBe(false);
    },
  );

  it("is disabled while the cash is recorded", () => {
    expect(saveLabel(SENT, "1.000.000", true)).toBe(SAVING_TEXT);
    expect(canSave(SENT, true)).toBe(false);
  });
});

describe("photo lines", () => {
  it("shows the percent once it is known", () => {
    expect(uploadingLine(null)).toBe("Mengirim foto…");
    expect(uploadingLine(64)).toBe("Mengirim foto… 64%");
  });

  it("announces state changes, never each percent", () => {
    expect(
      photoAnnouncement({ kind: "uploading", photo: null, percent: 10 }),
    ).toBe(PHOTO_SENDING_TEXT);
    expect(
      photoAnnouncement({ kind: "uploading", photo: PHOTO, percent: 90 }),
    ).toBe(PHOTO_SENDING_TEXT);
    expect(photoAnnouncement(SENT)).toBe("Foto terkirim.");
    expect(
      photoAnnouncement({
        kind: "failed",
        photo: PHOTO,
        reason: "network",
        message: PHOTO_NETWORK_TEXT,
        resume: { step: "create" },
      }),
    ).toBe("Foto belum terkirim. Sinyal hilang. Uang belum tercatat.");
    expect(photoAnnouncement({ kind: "empty" })).toBe("");
  });

  it("ends another reason with 'Uang belum tercatat.'", () => {
    expect(notRecordedLine("Penyimpanan foto belum tersedia")).toBe(
      "Penyimpanan foto belum tersedia. Uang belum tercatat.",
    );
  });
});

describe("uploadFailureView", () => {
  it("shows nothing for a cancelled PUT", () => {
    expect(uploadFailureView(new StorageUploadError("aborted"))).toEqual({
      kind: "cancelled",
    });
  });

  it.each([
    ["a PUT with no answer", new StorageUploadError("network")],
    [
      "an API call with no answer",
      apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung"),
    ],
  ])("is 'Sinyal hilang' for %s", (_, error) => {
    expect(uploadFailureView(error)).toEqual({
      kind: "failed",
      reason: "network",
      message: PHOTO_NETWORK_TEXT,
    });
  });

  it("keeps Coba lagi when storage refuses the PUT (an expired URL)", () => {
    expect(uploadFailureView(new StorageUploadError("refused", 403))).toEqual({
      kind: "failed",
      reason: "other",
      message: "Penyimpanan foto menolak kiriman. Uang belum tercatat.",
    });
  });

  it.each([
    apiError(
      400,
      "upload_type_not_allowed",
      "Foto harus JPG, PNG atau WebP.",
      "contentType",
    ),
    apiError(
      400,
      "upload_too_large",
      "Ukuran foto paling besar 10 MB.",
      "size",
    ),
    apiError(400, "invalid_input", "Ukuran foto tidak valid.", "size"),
  ])("puts the API's refusal of the file under the photo ($code)", (error) => {
    expect(uploadFailureView(error)).toEqual({
      kind: "refused",
      message: error.message,
    });
  });

  it("keeps Coba lagi with the API's message for other refusals", () => {
    expect(
      uploadFailureView(
        apiError(503, "uploads_unavailable", "Unggah foto belum tersedia."),
      ),
    ).toEqual({
      kind: "failed",
      reason: "other",
      message: "Unggah foto belum tersedia. Uang belum tercatat.",
    });
  });

  it("is generic for an error that is not the API's", () => {
    expect(uploadFailureView(new TypeError("boom"))).toEqual({
      kind: "failed",
      reason: "other",
      message: notRecordedLine(GENERIC_ERROR_MESSAGE),
    });
  });
});

describe("classifyUploadFailure", () => {
  it.each([
    [new StorageUploadError("aborted"), { kind: "cancelled" }],
    [new StorageUploadError("network"), { kind: "network" }],
    [
      apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung"),
      { kind: "network" },
    ],
    [
      new StorageUploadError("refused", 403),
      { kind: "other", message: "Penyimpanan foto menolak kiriman." },
    ],
    [
      apiError(400, "upload_too_large", "Ukuran foto paling besar 10 MB."),
      { kind: "refused", message: "Ukuran foto paling besar 10 MB." },
    ],
    [
      apiError(403, "forbidden", "Anda tidak punya akses."),
      { kind: "denied", message: "Anda tidak punya akses." },
    ],
    [
      apiError(404, "not_found", "Penyewa tidak ditemukan.", "tenantId"),
      { kind: "denied", message: "Penyewa tidak ditemukan." },
    ],
    [
      apiError(503, "uploads_unavailable", "Unggah foto belum tersedia."),
      { kind: "other", message: "Unggah foto belum tersedia." },
    ],
    [new TypeError("boom"), { kind: "other", message: GENERIC_ERROR_MESSAGE }],
  ])("classifies %s", (error, want) => {
    expect(classifyUploadFailure(error)).toEqual(want);
  });

  it("keeps Coba lagi on Terima tunai when the API refuses the member", () => {
    expect(
      uploadFailureView(apiError(403, "forbidden", "Anda tidak punya akses.")),
    ).toEqual({
      kind: "failed",
      reason: "other",
      message: "Anda tidak punya akses. Uang belum tercatat.",
    });
  });
});

describe("recordErrorView", () => {
  it.each([
    apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung"),
    apiError(502, "unknown", GENERIC_ERROR_MESSAGE),
  ])(
    "may have recorded the cash when no answer came (status $status)",
    (error) => {
      expect(recordErrorView(error)).toEqual({
        kind: "unknown",
        message: UNKNOWN_SAVE_TEXT,
      });
    },
  );

  it.each([
    apiError(
      409,
      "receipt_photo_upload_refused",
      "Foto ditolak pemeriksaan. Ambil foto lain.",
      "photoUploadId",
    ),
    apiError(
      400,
      "receipt_photo_upload_invalid",
      "Foto tidak bisa dipakai.",
      "photoUploadId",
    ),
    apiError(404, "not_found", "Foto tidak ditemukan.", "photoUploadId"),
  ])("puts a refused photo under the photo ($code)", (error) => {
    expect(recordErrorView(error)).toEqual({
      kind: "photo",
      message: error.message,
    });
  });

  it("puts the amount's error under the amount", () => {
    expect(
      recordErrorView(
        apiError(400, "invalid_input", "Jumlah harus lebih dari 0.", "amount"),
      ),
    ).toEqual({ kind: "amount", message: "Jumlah harus lebih dari 0." });
  });

  it("is a generic alert for request_id_reused", () => {
    expect(
      recordErrorView(
        apiError(409, "request_id_reused", "requestId dipakai.", "requestId"),
      ),
    ).toEqual({ kind: "alert", message: GENERIC_ERROR_MESSAGE });
  });

  it("shows the API's message for a denial, and the sheet stays", () => {
    expect(
      recordErrorView(
        apiError(403, "forbidden", "Anda tidak boleh mencatat tunai ini."),
      ),
    ).toEqual({
      kind: "alert",
      message: "Anda tidak boleh mencatat tunai ini.",
    });
  });

  it("is generic for an error that is not the API's", () => {
    expect(recordErrorView(new TypeError("boom"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});

describe("Tersimpan", () => {
  it.each([
    ["pending", "Kwitansi sedang dikirim ke WA Bu Wati Handayani."],
    ["sent", "Kwitansi dikirim ke WA Bu Wati Handayani."],
    ["skipped", "Kwitansi tidak dikirim."],
  ] as const)("says the receipt is %s", (status, line) => {
    expect(receiptLine(status, "Bu Wati Handayani")).toBe(line);
  });

  it("does not wait for the scan", () => {
    expect(photoLine("pending")).toEqual({
      title: "Foto kwitansi tersimpan",
      detail: "Sedang diperiksa sistem",
      refused: false,
    });
    expect(photoLine("infected").refused).toBe(true);
  });

  const result: CashPaymentResult = {
    outstanding: 50_000,
    overdue: 50_000,
    credit: 0,
    oldestUnpaidDueDate: "2026-11-09",
    payment: {
      id: "5d7e9f10-2a3b-4c5d-8e6f-7a8b9c0d1e2f",
      amount: 1_000_000,
      net: 1_000_000,
      method: "cash",
      receivedOn: "2026-11-16",
      note: "",
      allocations: [],
      corrections: [],
      createdAt: "2026-11-16T03:12:00Z",
    },
    paid: [
      {
        invoiceId: "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
        kind: "regular",
        period: "2026-11-09",
        periodEnd: "2026-12-08",
        dueDate: "2026-11-09",
        amount: 1_000_000,
        outstanding: 50_000,
      },
    ],
    receipt: { status: "pending" },
    photo: { status: "pending", uploadId: SENT.uploadId },
  };

  it("shows the API's answer as it is", () => {
    expect(cashRecordedView(result, "Bu Wati Handayani")).toEqual({
      headline: "Rp1.000.000 tercatat",
      receiptLine: "Kwitansi sedang dikirim ke WA Bu Wati Handayani.",
      paid: [
        {
          invoiceId: "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
          name: "Tagihan November 2026",
          amount: "Rp1.000.000",
        },
      ],
      paidEmptyText: null,
      totals: [{ label: "Sisa yang belum dibayar", value: "Rp50.000" }],
      photo: {
        title: "Foto kwitansi tersimpan",
        detail: "Sedang diperiksa sistem",
        refused: false,
      },
    });
  });

  it("says it all went to credit when it paid no invoice", () => {
    const view = cashRecordedView(
      { ...result, paid: [], outstanding: 0, overdue: 0, credit: 200_000 },
      "Bu Wati Handayani",
    );
    expect(view.paidEmptyText).toBe(ALL_CREDIT_TEXT);
    expect(view.totals).toEqual([
      { label: "Saldo lebih bayar", value: "Rp200.000" },
      { label: "Sisa yang belum dibayar", value: "Rp0" },
    ]);
  });
});

describe("uploadTenantPhoto", () => {
  const UPLOAD_ID = "3c9d0e1f-2a4b-4c6d-8e0f-1a2b3c4d5e6f";
  const TICKET: UploadTicket = {
    upload: {
      id: UPLOAD_ID,
      purpose: "cash_receipt",
      status: "pending",
      size: 10,
      createdAt: "2026-11-16T03:10:00Z",
    },
    putUrl: "https://storage.example.test/put",
    putHeaders: { "Content-Type": "image/jpeg" },
    putExpiresAt: "2026-11-16T03:15:00Z",
  };

  function fakeSteps(overrides: Partial<PhotoUploadSteps> = {}) {
    const calls: string[] = [];
    const steps: PhotoUploadSteps = {
      create: vi.fn(async (input) => {
        calls.push(`create ${input.purpose} ${input.size}`);
        return TICKET;
      }),
      put: vi.fn(async (_ticket, _file, options) => {
        calls.push("put");
        options.onProgress?.({ loaded: 5, total: 10 });
      }),
      complete: vi.fn(async (uploadId) => {
        calls.push(`complete ${uploadId}`);
        return undefined;
      }),
      ...overrides,
    };
    return { steps, calls };
  }

  function run(
    steps: PhotoUploadSteps,
    resume: Parameters<typeof uploadTenantPhoto>[1]["resume"] = {
      step: "create",
    },
    signal = new AbortController().signal,
  ) {
    return uploadTenantPhoto(steps, {
      purpose: "cash_receipt",
      tenantId: "tenant-1",
      file: PHOTO.file,
      resume,
      signal,
      onProgress: () => undefined,
    });
  }

  it("creates the upload, PUTs the file, then completes it", async () => {
    const { steps, calls } = fakeSteps();
    await expect(run(steps)).resolves.toEqual({
      kind: "sent",
      uploadId: UPLOAD_ID,
    });
    expect(calls).toEqual([
      "create cash_receipt 10",
      "put",
      `complete ${UPLOAD_ID}`,
    ]);
  });

  it("creates the upload for the purpose it is given", async () => {
    const { steps, calls } = fakeSteps();
    await uploadTenantPhoto(steps, {
      purpose: "transfer_proof",
      tenantId: "tenant-1",
      file: PHOTO.file,
      resume: { step: "create" },
      signal: new AbortController().signal,
      onProgress: () => undefined,
    });
    expect(calls).toEqual([
      "create transfer_proof 10",
      "put",
      `complete ${UPLOAD_ID}`,
    ]);
  });

  it("passes the PUT the ticket, the file, the signal and the progress", async () => {
    const { steps } = fakeSteps();
    const controller = new AbortController();
    const onProgress = vi.fn();
    await uploadTenantPhoto(steps, {
      purpose: "cash_receipt",
      tenantId: "tenant-1",
      file: PHOTO.file,
      resume: { step: "create" },
      signal: controller.signal,
      onProgress,
    });
    expect(steps.put).toHaveBeenCalledWith(TICKET, PHOTO.file, {
      signal: controller.signal,
      onProgress,
    });
    expect(onProgress).toHaveBeenCalledWith({ loaded: 5, total: 10 });
  });

  it("starts again from a new upload when the PUT fails", async () => {
    const failure = new StorageUploadError("network");
    const { steps, calls } = fakeSteps({
      put: vi.fn(async () => {
        throw failure;
      }),
    });
    await expect(run(steps)).resolves.toEqual({
      kind: "failed",
      error: failure,
      resume: { step: "create" },
    });
    expect(calls).toEqual(["create cash_receipt 10"]);
  });

  it("starts again from completeUpload when only that failed", async () => {
    const failure = apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung");
    const { steps } = fakeSteps({
      complete: vi.fn(async () => {
        throw failure;
      }),
    });
    await expect(run(steps)).resolves.toEqual({
      kind: "failed",
      error: failure,
      resume: { step: "complete", uploadId: UPLOAD_ID },
    });
  });

  it("resumes with completeUpload alone, never sending the file twice", async () => {
    const { steps, calls } = fakeSteps();
    await expect(
      run(steps, { step: "complete", uploadId: UPLOAD_ID }),
    ).resolves.toEqual({ kind: "sent", uploadId: UPLOAD_ID });
    expect(calls).toEqual([`complete ${UPLOAD_ID}`]);
  });

  it("drops what a cancelled attempt answers, and sends nothing after it", async () => {
    const controller = new AbortController();
    const { steps, calls } = fakeSteps({
      create: vi.fn(async () => {
        controller.abort();
        return TICKET;
      }),
    });
    await expect(
      run(steps, { step: "create" }, controller.signal),
    ).resolves.toEqual({ kind: "cancelled" });
    expect(calls).toEqual([]);
    expect(steps.put).not.toHaveBeenCalled();
    expect(steps.complete).not.toHaveBeenCalled();
  });

  it("is cancelled, not failed, when the PUT is aborted", async () => {
    const controller = new AbortController();
    const { steps } = fakeSteps({
      put: vi.fn(async () => {
        controller.abort();
        throw new StorageUploadError("aborted");
      }),
    });
    await expect(
      run(steps, { step: "create" }, controller.signal),
    ).resolves.toEqual({ kind: "cancelled" });
  });

  it("is cancelled when completeUpload answers after the attempt was stopped", async () => {
    const controller = new AbortController();
    const { steps } = fakeSteps({
      complete: vi.fn(async () => {
        controller.abort();
        return undefined;
      }),
    });
    await expect(
      run(steps, { step: "create" }, controller.signal),
    ).resolves.toEqual({ kind: "cancelled" });
  });
});
