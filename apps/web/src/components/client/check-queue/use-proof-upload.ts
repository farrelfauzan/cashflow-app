"use client";

import { useEffect, useEffectEvent } from "react";

import { toFormError } from "@/lib/api/form-error";
import { useGetUpload } from "@/lib/api/generated/uploads/uploads";
import type { CheckTransferProof } from "@/lib/api/generated/model";

import type { ProofPhotoState } from "./type";
import { proofPhotoState, proofReady, uploadRefetchMs } from "./utils";

type ProofUploadOptions = {
  orgId: string;
  proof: CheckTransferProof;
  // The drawer is open: only then is the photo read.
  enabled: boolean;
  // The photo the queue listed as being scanned is now clean.
  onCleaned: () => void;
  // The scan refused the photo while the drawer was open.
  onRefused: () => void;
};

type ProofUpload = {
  photo: ProofPhotoState;
  // The scan found the photo clean: "Simpan pembayaran" may go.
  ready: boolean;
  // Asks getUpload again: after the image failed to load (an expired link) or the API said the
  // photo is not ready.
  reload: () => void;
};

// The proof's photo from getUpload. Its download link lasts a few minutes, so it is never kept:
// the query is dropped as soon as the drawer closes (gcTime 0), read afresh on every opening, and
// read again shortly before the link expires. While the photo is being scanned it is read every
// few seconds, so the drawer turns usable without a reload.
export function useProofUpload({
  orgId,
  proof,
  enabled,
  onCleaned,
  onRefused,
}: ProofUploadOptions): ProofUpload {
  const upload = useGetUpload(orgId, proof.uploadId, {
    query: {
      enabled,
      staleTime: 0,
      gcTime: 0,
      refetchInterval: (query) => uploadRefetchMs(query.state.data, Date.now()),
    },
  });

  const status = upload.data?.status;
  const statusChanged = useEffectEvent((next: typeof status) => {
    if (next === "infected" || next === "rejected") onRefused();
    if (next === "clean" && proof.uploadStatus === "pending") onCleaned();
  });
  useEffect(() => {
    statusChanged(status);
  }, [status]);

  const errorMessage =
    upload.error === null ? null : toFormError(upload.error, []).message;
  return {
    photo: proofPhotoState(upload.data, proof.uploadStatus, errorMessage),
    ready: proofReady(upload.data, proof.uploadStatus),
    reload: () => void upload.refetch(),
  };
}
