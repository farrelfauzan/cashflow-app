"use client";

import { keepPreviousData } from "@tanstack/react-query";

import { useDebouncedValue } from "@/components/client/form/use-debounced-value";
import { toFormError } from "@/lib/api/form-error";
import { usePreviewTenantPayment } from "@/lib/api/generated/money/money";

import type { ProofPreviewView } from "./type";
import { PREVIEW_DEBOUNCE_MS, proofPreviewView } from "./utils";

type ProofPreviewOptions = {
  orgId: string;
  tenantId: string;
  // The field's valid amount, or null while it holds none.
  amount: number | null;
};

type ProofPreview = {
  // The last answer for a valid amount; while a newer amount loads, the previous one stays.
  view: ProofPreviewView | null;
  // The field changed and the debounce has not ended, or a request is on its way.
  loading: boolean;
  // The API's refusal of the read for the field's amount, in Bahasa Indonesia.
  error: string | null;
};

// What recording the typed amount would pay, from previewTenantPayment, read once typing has
// paused. Nothing is computed here: the lines are the API's answer, only formatted.
export function useProofPreview({
  orgId,
  tenantId,
  amount,
}: ProofPreviewOptions): ProofPreview {
  const debounced = useDebouncedValue(amount, PREVIEW_DEBOUNCE_MS);
  const settled = debounced === amount;
  const preview = usePreviewTenantPayment(
    orgId,
    tenantId,
    { amount: debounced ?? 0 },
    {
      query: {
        enabled: debounced !== null,
        retry: false,
        refetchOnWindowFocus: false,
        placeholderData: keepPreviousData,
      },
    },
  );

  if (amount === null) return { view: null, loading: false, error: null };
  const error =
    settled && preview.isError ? toFormError(preview.error, []).message : null;
  const data = preview.isError ? undefined : preview.data;
  return {
    view: data === undefined ? null : proofPreviewView(data),
    loading: !settled || preview.isFetching,
    error,
  };
}
