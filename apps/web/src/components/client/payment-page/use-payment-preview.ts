"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { useEffect } from "react";

import { useDebouncedValue } from "@/components/client/form/use-debounced-value";
import { useGetPaymentPreview } from "@/lib/api/generated/payment-page/payment-page";

import type { PreviewErrorView, PreviewView } from "./type";
import { PREVIEW_DEBOUNCE_MS, previewErrorView, previewView } from "./utils";

type UsePaymentPreviewInput = {
  // The link's token: it goes only into the generated request's path.
  token: string;
  // The field's valid amount, or null while there is none.
  amount: number | null;
  onLinkGone: () => void;
};

type PaymentPreviewState = {
  // The last answer for a valid amount; while a new amount loads, the previous one stays.
  view: PreviewView | null;
  // The answer is for exactly the field's amount: the primary may go.
  ready: boolean;
  // A request is on its way, or the field changed and the debounce has not ended.
  loading: boolean;
  // The failure of the read for the field's amount.
  error: PreviewErrorView | null;
};

// What the field's amount would pay, from getPaymentPreview. Read once typing has paused
// (PREVIEW_DEBOUNCE_MS), only for a valid amount, never retried or polled: it shares the page's
// rate limit per token. A link that stopped working replaces the page, as for the other reads.
export function usePaymentPreview({
  token,
  amount,
  onLinkGone,
}: UsePaymentPreviewInput): PaymentPreviewState {
  const debounced = useDebouncedValue(amount, PREVIEW_DEBOUNCE_MS);
  const settled = debounced === amount;
  const preview = useGetPaymentPreview(
    token,
    { amount: debounced ?? 0 },
    {
      query: {
        enabled: debounced !== null,
        retry: false,
        refetchOnWindowFocus: false,
        refetchInterval: false,
        placeholderData: keepPreviousData,
      },
    },
  );

  const error =
    settled && debounced !== null && preview.isError
      ? previewErrorView(preview.error)
      : null;
  const linkGone = error?.kind === "link-gone";
  useEffect(() => {
    if (linkGone) onLinkGone();
  }, [linkGone, onLinkGone]);

  if (amount === null) {
    return { view: null, ready: false, loading: false, error: null };
  }
  const data = preview.isError ? undefined : preview.data;
  return {
    view: data === undefined ? null : previewView(data),
    ready:
      settled &&
      data !== undefined &&
      !preview.isPlaceholderData &&
      data.amount === amount,
    loading: !settled || preview.isFetching,
    error,
  };
}
