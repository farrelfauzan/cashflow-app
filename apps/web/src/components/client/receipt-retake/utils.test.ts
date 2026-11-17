import { describe, expect, it } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type { ReceiptPhotoStatus } from "@/lib/api/generated/model";
import { StorageUploadError } from "@/lib/api/storage-upload";

import {
  RETAKE_NETWORK_TEXT,
  RETAKE_SAVE_TEXT,
  RETAKE_SAVING_TEXT,
  RETAKE_UNKNOWN_TEXT,
  RETAKE_WAITING_TEXT,
  attachErrorView,
  attachPhotoBody,
  canRetakePhoto,
  canSaveRetake,
  notNeededToast,
  retakeButtonLabel,
  retakePayment,
  retakeSaveLabel,
  retakeSummary,
  retakeUploadFailureView,
  retakenToast,
} from "./utils";

function apiError(
  status: number,
  code: string,
  message: string,
  field?: string,
): ApiError {
  return new ApiError({ status, code, message, field });
}

describe("canRetakePhoto", () => {
  it.each<[ReceiptPhotoStatus, boolean]>([
    ["rejected", true],
    ["infected", true],
    ["clean", false],
    ["pending", false],
    ["missing", false],
  ])("%s gives %s", (status, want) => {
    expect(canRetakePhoto(status)).toBe(want);
  });
});

describe("retakePayment", () => {
  it("takes the payment, its tenant and the place", () => {
    expect(
      retakePayment(
        {
          paymentId: "p-1",
          amount: 1_000_000,
          recordedAt: "2026-11-16T03:42:00Z",
          photo: { status: "infected", uploadId: "u-1" },
        },
        { tenantId: "t-1", tenantName: "Bu Wati Handayani" },
        "Kamar 11 · Kos Melati",
      ),
    ).toEqual({
      paymentId: "p-1",
      tenantId: "t-1",
      tenantName: "Bu Wati Handayani",
      place: "Kamar 11 · Kos Melati",
      amount: 1_000_000,
      recordedAt: "2026-11-16T03:42:00Z",
      photoStatus: "infected",
    });
  });
});

describe("retakeSummary and retakeButtonLabel", () => {
  // Senin 16 Nov 2026, 14.00 in Jakarta.
  const NOW = new Date("2026-11-16T07:00:00Z");

  it("shows only the time for cash recorded today in Jakarta", () => {
    expect(retakeSummary(1_000_000, "2026-11-16T03:42:00Z", NOW)).toBe(
      "Tunai Rp1.000.000 · 10.42",
    );
  });

  it("counts the day in Jakarta, not UTC", () => {
    // 15 Nov 17:30 UTC is 16 Nov 00.30 in Jakarta.
    expect(retakeSummary(50_000, "2026-11-15T17:30:00Z", NOW)).toBe(
      "Tunai Rp50.000 · 00.30",
    );
  });

  it("names the day of cash recorded earlier", () => {
    expect(retakeSummary(700_000, "2026-11-13T03:15:00Z", NOW)).toBe(
      "Tunai Rp700.000 · Jumat 13 Nov, 10.15",
    );
  });

  it("starts the button's name with its visible text", () => {
    expect(retakeButtonLabel("Tunai Rp1.000.000 · 10.42")).toBe(
      "Ambil ulang foto kwitansi Tunai Rp1.000.000 · 10.42",
    );
  });
});

describe("retakeUploadFailureView", () => {
  it("shows nothing for a cancelled PUT", () => {
    expect(retakeUploadFailureView(new StorageUploadError("aborted"))).toEqual({
      kind: "cancelled",
    });
  });

  it("is 'Sinyal hilang' without saying the cash is unrecorded", () => {
    expect(retakeUploadFailureView(new StorageUploadError("network"))).toEqual({
      kind: "failed",
      reason: "network",
      message: RETAKE_NETWORK_TEXT,
    });
  });

  it("puts the API's refusal of the file under the photo", () => {
    expect(
      retakeUploadFailureView(
        apiError(400, "upload_too_large", "Ukuran foto paling besar 10 MB."),
      ),
    ).toEqual({ kind: "refused", message: "Ukuran foto paling besar 10 MB." });
  });

  it("keeps Coba lagi with the API's message for other refusals", () => {
    expect(
      retakeUploadFailureView(
        apiError(503, "uploads_unavailable", "Unggah foto belum tersedia"),
      ),
    ).toEqual({
      kind: "failed",
      reason: "other",
      message: "Unggah foto belum tersedia. Foto belum diganti.",
    });
  });
});

describe("attachPhotoBody", () => {
  it("sends the completed upload's id", () => {
    expect(attachPhotoBody("u-1")).toEqual({ uploadId: "u-1" });
  });
});

describe("attachErrorView", () => {
  it.each([
    apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung"),
    apiError(502, "unknown", GENERIC_ERROR_MESSAGE),
  ])("may have attached the photo when no answer came ($status)", (error) => {
    expect(attachErrorView(error)).toEqual({
      kind: "unknown",
      message: RETAKE_UNKNOWN_TEXT,
    });
  });

  it("knows the photo is no longer refused", () => {
    expect(
      attachErrorView(
        apiError(
          409,
          "receipt_photo_attached",
          "Pembayaran ini sudah punya foto.",
        ),
      ),
    ).toEqual({
      kind: "attached",
      message: "Pembayaran ini sudah punya foto.",
    });
  });

  it.each([
    apiError(
      409,
      "receipt_photo_upload_refused",
      "Foto ini ditolak saat diperiksa. Ambil foto lagi.",
      "uploadId",
    ),
    apiError(
      400,
      "receipt_photo_upload_invalid",
      "Foto ini tidak bisa dipakai untuk pembayaran ini. Ambil foto lagi.",
      "uploadId",
    ),
    apiError(404, "not_found", "Tidak ditemukan.", "uploadId"),
  ])("puts the new photo's refusal under the photo ($code)", (error) => {
    expect(attachErrorView(error)).toEqual({
      kind: "photo",
      message: error.message,
    });
  });

  it.each([
    apiError(403, "forbidden", "Anda tidak boleh mengganti foto ini."),
    apiError(404, "not_found", "Tidak ditemukan."),
  ])("shows the API's message at the top ($status)", (error) => {
    expect(attachErrorView(error)).toEqual({
      kind: "alert",
      message: error.message,
    });
  });

  it("is generic for an error that is not the API's", () => {
    expect(attachErrorView(new TypeError("boom"))).toEqual({
      kind: "alert",
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});

describe("retakeSaveLabel and canSaveRetake", () => {
  it("works once the photo has arrived", () => {
    expect(retakeSaveLabel("sent", false)).toBe(RETAKE_SAVE_TEXT);
    expect(canSaveRetake("sent", false)).toBe(true);
  });

  it("waits while the photo is sent", () => {
    expect(retakeSaveLabel("uploading", false)).toBe(RETAKE_WAITING_TEXT);
    expect(canSaveRetake("uploading", false)).toBe(false);
  });

  it.each(["empty", "failed", "refused"] as const)(
    "is disabled without a sent photo (%s)",
    (kind) => {
      expect(retakeSaveLabel(kind, false)).toBe(RETAKE_SAVE_TEXT);
      expect(canSaveRetake(kind, false)).toBe(false);
    },
  );

  it("is disabled while the photo is attached", () => {
    expect(retakeSaveLabel("sent", true)).toBe(RETAKE_SAVING_TEXT);
    expect(canSaveRetake("sent", true)).toBe(false);
  });
});

describe("toasts", () => {
  it("names the tenant when the photo is replaced", () => {
    expect(retakenToast("Bu Wati Handayani")).toEqual({
      tone: "create",
      title: "Foto kwitansi diganti",
      description: "Bu Wati Handayani",
    });
  });

  it("says why nothing was retaken, in the API's words", () => {
    expect(notNeededToast("Pembayaran ini sudah punya foto.")).toEqual({
      tone: "neutral",
      title: "Foto tidak perlu diganti",
      description: "Pembayaran ini sudah punya foto.",
    });
  });
});
