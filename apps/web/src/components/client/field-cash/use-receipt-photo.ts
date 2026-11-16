"use client";

import { useEffect, useRef, useState } from "react";

import { uploadPercent } from "@/components/client/payment-page/utils";
import {
  useCompleteUpload,
  useCreateUpload,
} from "@/lib/api/generated/uploads/uploads";
import { putToStorage } from "@/lib/api/storage-upload";
import { isTooLarge, preparePhoto } from "@/lib/photo";

import type {
  ChosenPhoto,
  PhotoResume,
  ReceiptPhotoState,
  ReceiptPhotoUpload,
  ReceiptUploadSteps,
} from "./type";
import {
  PHOTO_TOO_LARGE_TEXT,
  uploadFailureView,
  uploadReceiptPhoto,
} from "./utils";

// "Foto kwitansi (wajib)": the receipt photo goes to storage before the cash can be saved
// (decision 50). A chosen photo is made ready (lib/photo: HEIC to JPEG, at most 2000px on its
// longest side, 10 MB at most), then sent at once by uploadReceiptPhoto (createUpload, the PUT
// with its percent, completeUpload); the scan is not waited for. Each attempt has its own
// AbortController: choosing another photo, "Coba lagi", Batal or leaving aborts the running one,
// which stops its PUT and drops whatever it answers after. The object URL that shows the photo
// is revoked when it is replaced or the sheet goes.
export function useReceiptPhoto(
  orgId: string,
  tenantId: string,
): ReceiptPhotoUpload {
  const [state, setState] = useState<ReceiptPhotoState>({ kind: "empty" });
  const createUpload = useCreateUpload();
  const completeUpload = useCompleteUpload();
  // The running attempt (preparing or sending), null when none runs.
  const attemptRef = useRef<AbortController | null>(null);
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

  // Stops the running attempt and starts a new one.
  function nextAttempt(): AbortSignal {
    attemptRef.current?.abort();
    const controller = new AbortController();
    attemptRef.current = controller;
    return controller.signal;
  }

  function stop() {
    attemptRef.current?.abort();
    attemptRef.current = null;
  }

  function show(file: File): ChosenPhoto {
    if (urlRef.current !== null) URL.revokeObjectURL(urlRef.current);
    const previewUrl = URL.createObjectURL(file);
    urlRef.current = previewUrl;
    return { file, previewUrl };
  }

  const steps: ReceiptUploadSteps = {
    create: (data) => createUpload.mutateAsync({ orgId, data }),
    put: putToStorage,
    complete: (uploadId) => completeUpload.mutateAsync({ orgId, uploadId }),
  };

  async function send(
    signal: AbortSignal,
    photo: ChosenPhoto,
    resume: PhotoResume,
  ) {
    setState({ kind: "uploading", photo, percent: null });
    const outcome = await uploadReceiptPhoto(steps, {
      tenantId,
      file: photo.file,
      resume,
      signal,
      onProgress: ({ loaded, total }) => {
        if (signal.aborted) return;
        const percent = uploadPercent(loaded, total);
        setState((current) =>
          current.kind === "uploading" ? { ...current, percent } : current,
        );
      },
    });
    if (outcome.kind === "cancelled") return;
    attemptRef.current = null;
    if (outcome.kind === "sent") {
      setState({ kind: "sent", photo, uploadId: outcome.uploadId });
      return;
    }
    const view = uploadFailureView(outcome.error);
    if (view.kind === "cancelled") return;
    if (view.kind === "refused") {
      setState({ kind: "refused", photo, message: view.message });
      return;
    }
    setState({
      kind: "failed",
      photo,
      reason: view.reason,
      message: view.message,
      resume: outcome.resume,
    });
  }

  function choose(file: File) {
    const signal = nextAttempt();
    setState({ kind: "uploading", photo: null, percent: null });
    void preparePhoto(file).then((prepared) => {
      if (signal.aborted) return;
      const photo = show(prepared);
      if (isTooLarge(prepared.size)) {
        attemptRef.current = null;
        setState({ kind: "refused", photo, message: PHOTO_TOO_LARGE_TEXT });
        return;
      }
      void send(signal, photo, { step: "create" });
    });
  }

  function retry() {
    // A second quick press finds the first attempt running and starts nothing.
    if (state.kind !== "failed" || attemptRef.current !== null) return;
    void send(nextAttempt(), state.photo, state.resume);
  }

  function refuse(message: string) {
    stop();
    setState((current) =>
      current.kind === "sent"
        ? { kind: "refused", photo: current.photo, message }
        : current,
    );
  }

  return { state, choose, retry, refuse, cancel: stop };
}
