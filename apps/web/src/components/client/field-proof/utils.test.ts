import { describe, expect, it } from "vitest";

import type { ChosenPhoto } from "@/components/client/field-cash/type";
import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  NETWORK_ERROR_CODE,
} from "@/lib/api/errors";
import type { CapabilityAccess } from "@/lib/api/generated/model";
import { StorageUploadError } from "@/lib/api/storage-upload";
import { createAppAbility } from "@/lib/capabilities";

import type { FieldProofState } from "./type";
import {
  FIELD_PROOF_NETWORK_TEXT,
  FIELD_PROOF_SEND_TEXT,
  canSendFieldProof,
  canSendFieldProofs,
  fieldProofUnsure,
  fieldProofFailureView,
  fieldProofPhoto,
  fieldProofPrimaryLabel,
  fieldProofSentToast,
} from "./utils";

const PHOTO: ChosenPhoto = {
  file: new File([new Uint8Array(10)], "bukti.jpg", { type: "image/jpeg" }),
  previewUrl: "blob:preview",
};

function apiError(
  status: number,
  code: string,
  message: string,
  field?: string,
): ApiError {
  return new ApiError({ status, code, message, field });
}

describe("canSendFieldProofs", () => {
  it.each<[CapabilityAccess, boolean]>([
    ["allow", true],
    ["conditional", true],
    ["deny", false],
  ])("payment:proof %s shows it: %s", (access, shown) => {
    const ability = createAppAbility([{ action: "payment:proof", access }]);
    expect(canSendFieldProofs(ability)).toBe(shown);
  });

  it("is hidden without payment:proof, whatever else the member may do", () => {
    const ability = createAppAbility([
      { action: "cash:record", access: "conditional" },
      { action: "promise:create", access: "conditional" },
    ]);
    expect(canSendFieldProofs(ability)).toBe(false);
  });
});

describe("fieldProofFailureView", () => {
  it.each([
    ["a PUT with no answer", new StorageUploadError("network")],
    [
      "an API call with no answer",
      apiError(0, NETWORK_ERROR_CODE, "Tidak dapat terhubung"),
    ],
  ])("is 'Sinyal hilang' with Coba lagi for %s", (_, error) => {
    expect(fieldProofFailureView(error)).toEqual({
      kind: "failed",
      message: FIELD_PROOF_NETWORK_TEXT,
    });
  });

  it("shows nothing for a PUT the sheet aborted", () => {
    expect(fieldProofFailureView(new StorageUploadError("aborted"))).toEqual({
      kind: "cancelled",
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
    expect(fieldProofFailureView(error)).toEqual({
      kind: "error",
      error: { kind: "file", message: error.message },
    });
  });

  it.each([
    apiError(403, "forbidden", "Anda tidak punya akses untuk tindakan ini."),
    apiError(404, "not_found", "Penyewa tidak ditemukan.", "tenantId"),
  ])(
    "shows the API's message with no retry when it refuses ($status)",
    (error) => {
      expect(fieldProofFailureView(error)).toEqual({
        kind: "error",
        error: { kind: "denied", message: error.message },
      });
    },
  );

  it("keeps Coba lagi with the API's message for other refusals", () => {
    expect(
      fieldProofFailureView(
        apiError(503, "uploads_unavailable", "Unggah foto belum tersedia."),
      ),
    ).toEqual({ kind: "failed", message: "Unggah foto belum tersedia." });
  });

  it("keeps Coba lagi when storage refuses the PUT (an expired URL)", () => {
    expect(
      fieldProofFailureView(new StorageUploadError("refused", 403)),
    ).toEqual({ kind: "failed", message: "Penyimpanan foto menolak kiriman." });
  });

  it("is generic for an error that is not the API's", () => {
    expect(fieldProofFailureView(new TypeError("boom"))).toEqual({
      kind: "failed",
      message: GENERIC_ERROR_MESSAGE,
    });
  });
});

describe("the sheet's state", () => {
  const states: Record<string, FieldProofState> = {
    empty: { kind: "empty" },
    preparing: { kind: "preparing" },
    ready: { kind: "ready", photo: PHOTO, error: null },
    refused: {
      kind: "ready",
      photo: PHOTO,
      error: { kind: "file", message: "Foto harus JPG, PNG atau WebP." },
    },
    denied: {
      kind: "ready",
      photo: PHOTO,
      error: { kind: "denied", message: "Tidak boleh." },
    },
    sending: { kind: "sending", photo: PHOTO, percent: 62 },
    failed: {
      kind: "failed",
      photo: PHOTO,
      message: FIELD_PROOF_NETWORK_TEXT,
      resume: { step: "create" },
    },
  };

  it.each(Object.keys(states))(
    "Kirim bukti works only when ready: %s",
    (key) => {
      expect(canSendFieldProof(states[key] as FieldProofState)).toBe(
        key === "ready",
      );
    },
  );

  it("shows the photo once one is chosen", () => {
    expect(fieldProofPhoto({ kind: "empty" })).toBeNull();
    expect(fieldProofPhoto({ kind: "preparing" })).toBeNull();
    expect(fieldProofPhoto(states.sending as FieldProofState)).toBe(PHOTO);
    expect(fieldProofPhoto(states.failed as FieldProofState)).toBe(PHOTO);
  });

  it("labels the primary Mengirim… only while sending", () => {
    expect(fieldProofPrimaryLabel(states.sending as FieldProofState)).toBe(
      "Mengirim…",
    );
    expect(fieldProofPrimaryLabel(states.failed as FieldProofState)).toBe(
      FIELD_PROOF_SEND_TEXT,
    );
    expect(fieldProofPrimaryLabel({ kind: "empty" })).toBe("Kirim bukti");
  });
});

describe("fieldProofSentToast", () => {
  it("says the admin will check it", () => {
    expect(fieldProofSentToast()).toEqual({
      tone: "create",
      title: "Bukti terkirim",
      description: "Admin akan mengeceknya.",
    });
  });
});

describe("fieldProofUnsure", () => {
  const photo = { file: new File(["x"], "bukti.jpg"), previewUrl: "blob:x" };

  it("is unsure only when completeUpload's answer was lost", () => {
    expect(
      fieldProofUnsure({
        kind: "failed",
        photo,
        message: "Sinyal hilang.",
        resume: { step: "complete", uploadId: "u1" },
      }),
    ).toBe(true);
  });

  it("is sure when the upload failed before completeUpload", () => {
    expect(
      fieldProofUnsure({
        kind: "failed",
        photo,
        message: "Sinyal hilang.",
        resume: { step: "create" },
      }),
    ).toBe(false);
  });

  it("is sure in every other state", () => {
    expect(fieldProofUnsure({ kind: "empty" })).toBe(false);
    expect(fieldProofUnsure({ kind: "ready", photo, error: null })).toBe(false);
    expect(fieldProofUnsure({ kind: "sending", photo, percent: 40 })).toBe(
      false,
    );
  });
});
