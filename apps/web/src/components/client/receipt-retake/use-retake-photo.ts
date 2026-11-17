"use client";

import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type FormEvent,
} from "react";

import { useReceiptPhoto } from "@/components/client/field-cash/use-receipt-photo";
import { useRefreshFieldCash } from "@/components/client/field-cash/use-refresh-field-cash";
import { notify } from "@/components/client/toast/notify";
import { useAttachCashPaymentPhoto } from "@/lib/api/generated/field/field";

import type { RetakePayment, RetakePhoto } from "./type";
import {
  attachErrorView,
  attachPhotoBody,
  notNeededToast,
  retakenToast,
  retakeUploadFailureView,
} from "./utils";

type RetakePhotoOptions = {
  orgId: string;
  payment: RetakePayment;
  // The photo is attached, or there is nothing left to retake: the sheet closes.
  onClose: () => void;
};

// One opening of "Ambil ulang foto kwitansi": the new photo goes to storage exactly as in Terima
// tunai (useReceiptPhoto: createUpload cash_receipt for the payment's tenant, the PUT with its
// percent, completeUpload), then "Simpan foto" sends its uploadId to attachCashPaymentPhoto.
//
// A lost answer locks the photo, so "Simpan foto" sends the same uploadId again (the API answers
// 200 and attaches it once). receipt_photo_attached means the payment's photo is no longer
// refused: a toast says so and the sheet closes. Tagihan hari ini, the cash held and the receipts
// held for Setor tunai are reloaded once the sheet closes after a photo was (or may have been)
// attached, or, if it goes while the request runs, once that settles.
export function useRetakePhoto({
  orgId,
  payment,
  onClose,
}: RetakePhotoOptions): RetakePhoto {
  const photo = useReceiptPhoto(
    orgId,
    payment.tenantId,
    retakeUploadFailureView,
  );
  const refresh = useRefreshFieldCash(orgId);
  const [alert, setAlert] = useState<string | null>(null);
  const [locked, setLocked] = useState(false);
  const [done, setDone] = useState(false);

  // Set the moment a request goes out, before React renders `saving`, so two quick presses send
  // once. Cleared when the API refuses.
  const sent = useRef(false);
  const pendingRef = useRef(false);
  // A photo was attached, or may have been: reload the lists when the sheet closes.
  const changedRef = useRef(false);
  const refreshOnSettleRef = useRef(false);

  // Mutation-level, so it runs even when the sheet is gone by the time the API answers.
  const attach = useAttachCashPaymentPhoto({
    mutation: {
      onSettled: () => {
        pendingRef.current = false;
        if (!refreshOnSettleRef.current) return;
        refreshOnSettleRef.current = false;
        void refresh();
      },
    },
  });
  const saving = attach.isPending;

  function finish() {
    photo.cancel();
    if (pendingRef.current) {
      refreshOnSettleRef.current = true;
      return;
    }
    if (!changedRef.current) return;
    changedRef.current = false;
    void refresh();
  }

  const onGone = useEffectEvent(finish);
  useEffect(() => () => onGone(), []);

  function refused(error: unknown) {
    sent.current = false;
    const view = attachErrorView(error);
    setLocked(view.kind === "unknown");
    switch (view.kind) {
      case "unknown":
        changedRef.current = true;
        setAlert(view.message);
        return;
      case "attached":
        changedRef.current = true;
        setAlert(null);
        setDone(true);
        notify(notNeededToast(view.message));
        onClose();
        return;
      case "photo":
        setAlert(null);
        photo.refuse(view.message);
        return;
      case "alert":
        setAlert(view.message);
        return;
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (sent.current || saving || done) return;
    if (photo.state.kind !== "sent") return;
    sent.current = true;
    pendingRef.current = true;
    attach.mutate(
      {
        orgId,
        paymentId: payment.paymentId,
        data: attachPhotoBody(photo.state.uploadId),
      },
      {
        onSuccess: () => {
          changedRef.current = true;
          setLocked(false);
          setAlert(null);
          setDone(true);
          notify(retakenToast(payment.tenantName));
          onClose();
        },
        onError: refused,
      },
    );
  }

  return {
    photo,
    alert,
    saving,
    locked,
    dirty: !done && photo.state.kind !== "empty",
    submit,
    finish,
  };
}
