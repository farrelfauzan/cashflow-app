"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import {
  getGetPaymentPageQueryKey,
  useSendPaymentProof,
} from "@/lib/api/generated/payment-page/payment-page";
import { preparePhoto } from "@/lib/photo";

import type {
  ProofPhoto,
  ProofSender,
  ProofSheetState,
  SendProof,
} from "./type";
import {
  canSendProof,
  proofErrorView,
  proofPhotoError,
  uploadPercent,
} from "./utils";

// "Kirim bukti transfer": the hidden file input, the sheet's state and one sendPaymentProof at a
// time. A chosen photo is made ready (shrunk, re-encoded to JPEG) and shown from an object URL,
// which is revoked when the photo is replaced, the sheet closes or the card goes. The sheet
// cannot be closed while a photo is sent. Once the API has the proof, the page is read again so
// "Bukti transfer terkirim" lists what the API says; nothing is added to it here.
export function useSendProof({
  token,
  onSent,
  onLinkGone,
}: ProofSender): SendProof {
  const queryClient = useQueryClient();
  const [state, setState] = useState<ProofSheetState>({ kind: "closed" });
  const inputRef = useRef<HTMLInputElement | null>(null);
  // The object URL on screen, revoked when it is replaced or the card goes.
  const urlRef = useRef<string | null>(null);
  // Counts each photo chosen and each close: a photo made ready after a newer one, or after the
  // sheet closed, is dropped.
  const pickRef = useRef(0);
  // A send is running: a second press sends nothing.
  const sendingRef = useRef(false);

  const mutation = useSendPaymentProof({
    request: {
      onUploadProgress: (event) => {
        const percent = uploadPercent(event.loaded, event.total);
        setState((current) =>
          current.kind === "sending" ? { ...current, percent } : current,
        );
      },
    },
    mutation: {
      // Raised here, not in mutate()'s callback, so the page is read again even if the card is
      // gone by the time the API answers. The send stays pending until that read ends, so the
      // sheet closes onto the new list.
      onSuccess: () =>
        queryClient.invalidateQueries({
          queryKey: getGetPaymentPageQueryKey(token),
        }),
    },
  });

  useEffect(
    () => () => {
      if (urlRef.current !== null) URL.revokeObjectURL(urlRef.current);
      urlRef.current = null;
    },
    [],
  );

  function showPhoto(file: File): ProofPhoto {
    if (urlRef.current !== null) URL.revokeObjectURL(urlRef.current);
    const previewUrl = URL.createObjectURL(file);
    urlRef.current = previewUrl;
    return { file, previewUrl, requestId: crypto.randomUUID() };
  }

  function close() {
    if (sendingRef.current) return;
    pickRef.current += 1;
    if (urlRef.current !== null) URL.revokeObjectURL(urlRef.current);
    urlRef.current = null;
    setState({ kind: "closed" });
  }

  function pickPhoto() {
    if (sendingRef.current) return;
    inputRef.current?.click();
  }

  function choose(file: File) {
    if (sendingRef.current) return;
    const pick = ++pickRef.current;
    mutation.reset();
    setState({ kind: "preparing" });
    void preparePhoto(file).then((prepared) => {
      if (pick !== pickRef.current) return;
      setState({
        kind: "ready",
        photo: showPhoto(prepared),
        error: proofPhotoError(prepared),
      });
    });
  }

  function send() {
    if (sendingRef.current || !canSendProof(state)) return;
    if (state.kind !== "ready") return;
    const { photo } = state;
    sendingRef.current = true;
    setState({ kind: "sending", photo, percent: 0 });
    mutation.mutate(
      {
        token,
        data: { file: photo.file },
        params: { requestId: photo.requestId },
      },
      {
        onSuccess: () => {
          sendingRef.current = false;
          close();
          onSent();
        },
        onError: (error) => {
          sendingRef.current = false;
          const view = proofErrorView(error);
          if (view.kind === "link-gone") {
            close();
            onLinkGone();
            return;
          }
          setState({ kind: "ready", photo, error: view });
        },
      },
    );
  }

  return { state, inputRef, pickPhoto, choose, send, close };
}
