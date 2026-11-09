"use client";

import { useRef, useState } from "react";

import { notify } from "@/components/client/toast/notify";
import { toFormError } from "@/lib/api/form-error";
import { useCreatePaymentLink } from "@/lib/api/generated/payment-page/payment-page";

import type { CopyPayLinkState } from "./type";
import {
  browserClipboard,
  browserClipboardItem,
  copyPendingText,
  copyToClipboard,
  PAY_LINK_COPIED_TOAST,
} from "./utils";

// "Salin link bayar": makes a new link (createPaymentLink, one per press, decision 39) and
// copies it. The link lives only in this hook's state, and only while the fallback shows it:
// the mutation is reset as soon as it answers and kept out of the cache (gcTime 0), and the link
// is never logged, put in a URL, a query key or storage, and the toast that confirms the copy
// never shows it.
export function useCopyPayLink(orgId: string): CopyPayLinkState {
  const mutation = useCreatePaymentLink({ mutation: { gcTime: 0 } });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fallbackUrl, setFallbackUrl] = useState<string | null>(null);
  // Blocks a second press before React renders the disabled button.
  const inFlight = useRef(false);

  async function run(invoiceId: string) {
    if (inFlight.current) return;
    inFlight.current = true;
    setBusy(true);
    setError(null);
    setFallbackUrl(null);
    // The copy starts now, inside the press (Safari needs that), with the link still on its way.
    const link = mutation
      .mutateAsync({ orgId, invoiceId })
      .then((created) => created.url);
    const copied = copyPendingText(
      browserClipboard(),
      browserClipboardItem(),
      link,
    );
    try {
      const url = await link;
      mutation.reset();
      if (await copied) {
        notify(PAY_LINK_COPIED_TOAST);
      } else {
        setFallbackUrl(url);
      }
    } catch (failed) {
      mutation.reset();
      setError(toFormError(failed, []).message);
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }

  async function copyAgain() {
    if (fallbackUrl === null) return;
    if (await copyToClipboard(browserClipboard(), fallbackUrl)) {
      setFallbackUrl(null);
      notify(PAY_LINK_COPIED_TOAST);
    }
  }

  return {
    busy,
    error,
    fallbackUrl,
    copy: (invoiceId) => void run(invoiceId),
    copyAgain: () => void copyAgain(),
    close: () => setFallbackUrl(null),
  };
}
