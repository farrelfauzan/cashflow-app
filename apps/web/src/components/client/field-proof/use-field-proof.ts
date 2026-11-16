"use client";

import { useEffect, useRef, useState } from "react";

import type {
  ChosenPhoto,
  PhotoResume,
  PhotoUploadSteps,
} from "@/components/client/field-cash/type";
import {
  PHOTO_TOO_LARGE_TEXT,
  uploadTenantPhoto,
} from "@/components/client/field-cash/utils";
import { uploadPercent } from "@/components/client/payment-page/utils";
import { notify } from "@/components/client/toast/notify";
import {
  useCompleteUpload,
  useCreateUpload,
} from "@/lib/api/generated/uploads/uploads";
import { putToStorage } from "@/lib/api/storage-upload";
import { isTooLarge, preparePhoto } from "@/lib/photo";

import type { FieldProofSend, FieldProofState } from "./type";
import { fieldProofFailureView, fieldProofSentToast } from "./utils";

type FieldProofOptions = {
  orgId: string;
  tenantId: string;
  // completeUpload answered: the sheet closes.
  onSent: () => void;
};

// One opening of "Bukti transfer dari penyewa". A chosen photo is made ready (lib/photo: HEIC to
// JPEG, at most 2000px, 10 MB at most) and shown; nothing is sent until "Kirim bukti". Then
// uploadTenantPhoto runs createUpload (transfer_proof for the tenant), the PUT with its percent
// and completeUpload, which records the proof for the admin. The scan is not waited for (a
// karyawan cannot read a transfer proof). One send at a time: a second press while one runs
// sends nothing. "Coba lagi" resumes where it failed: completeUpload again when only that failed
// (harmless to repeat), otherwise a new upload; the one before was never completed, so it never
// becomes a proof. Leaving aborts the PUT; the object URL is revoked when replaced or gone.
export function useFieldProof({
  orgId,
  tenantId,
  onSent,
}: FieldProofOptions): FieldProofSend {
  const [state, setState] = useState<FieldProofState>({ kind: "empty" });
  const createUpload = useCreateUpload();
  const completeUpload = useCompleteUpload();
  const inputRef = useRef<HTMLInputElement | null>(null);
  // The running attempt (preparing a photo or sending it), null when none runs.
  const attemptRef = useRef<AbortController | null>(null);
  // A send is running: no other photo, no second send, no close.
  const sendingRef = useRef(false);
  const urlRef = useRef<string | null>(null);

  useEffect(
    () => () => {
      attemptRef.current?.abort();
      attemptRef.current = null;
      if (urlRef.current !== null) URL.revokeObjectURL(urlRef.current);
      urlRef.current = null;
    },
    [],
  );

  function nextAttempt(): AbortSignal {
    attemptRef.current?.abort();
    const controller = new AbortController();
    attemptRef.current = controller;
    return controller.signal;
  }

  function show(file: File): ChosenPhoto {
    if (urlRef.current !== null) URL.revokeObjectURL(urlRef.current);
    const previewUrl = URL.createObjectURL(file);
    urlRef.current = previewUrl;
    return { file, previewUrl };
  }

  const steps: PhotoUploadSteps = {
    create: (data) => createUpload.mutateAsync({ orgId, data }),
    put: putToStorage,
    complete: (uploadId) => completeUpload.mutateAsync({ orgId, uploadId }),
  };

  async function run(photo: ChosenPhoto, resume: PhotoResume) {
    sendingRef.current = true;
    const signal = nextAttempt();
    setState({ kind: "sending", photo, percent: null });
    const outcome = await uploadTenantPhoto(steps, {
      purpose: "transfer_proof",
      tenantId,
      file: photo.file,
      resume,
      signal,
      onProgress: ({ loaded, total }) => {
        if (signal.aborted) return;
        const percent = uploadPercent(loaded, total);
        setState((current) =>
          current.kind === "sending" ? { ...current, percent } : current,
        );
      },
    });
    // The sheet went away mid-send.
    if (outcome.kind === "cancelled") return;
    sendingRef.current = false;
    attemptRef.current = null;
    if (outcome.kind === "sent") {
      notify(fieldProofSentToast());
      onSent();
      return;
    }
    const view = fieldProofFailureView(outcome.error);
    if (view.kind === "cancelled") {
      setState({ kind: "ready", photo, error: null });
      return;
    }
    if (view.kind === "error") {
      setState({ kind: "ready", photo, error: view.error });
      return;
    }
    setState({
      kind: "failed",
      photo,
      message: view.message,
      resume: outcome.resume,
    });
  }

  function pickPhoto() {
    if (sendingRef.current) return;
    inputRef.current?.click();
  }

  function choose(file: File) {
    if (sendingRef.current) return;
    const signal = nextAttempt();
    setState({ kind: "preparing" });
    void preparePhoto(file).then((prepared) => {
      if (signal.aborted) return;
      attemptRef.current = null;
      setState({
        kind: "ready",
        photo: show(prepared),
        // Refused before sending: the API would close the connection on a body this large.
        error: isTooLarge(prepared.size)
          ? { kind: "file", message: PHOTO_TOO_LARGE_TEXT }
          : null,
      });
    });
  }

  function send() {
    if (sendingRef.current || state.kind !== "ready" || state.error !== null) {
      return;
    }
    void run(state.photo, { step: "create" });
  }

  function retry() {
    if (sendingRef.current || state.kind !== "failed") return;
    void run(state.photo, state.resume);
  }

  return { state, inputRef, pickPhoto, choose, send, retry };
}
