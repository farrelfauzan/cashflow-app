"use client";

import { useQueryClient } from "@tanstack/react-query";

import { getListCashDepositsQueryKey } from "@/lib/api/generated/money/money";

// After a count, or when the API says the deposit was already counted: reload every list of
// deposits (both tabs, every page, and Penagihan's count). Cached money is never patched; the
// API's answer is the one shown.
export function useRefreshDeposits(orgId: string): () => Promise<void> {
  const queryClient = useQueryClient();
  return () =>
    queryClient.invalidateQueries({
      queryKey: getListCashDepositsQueryKey(orgId),
    });
}
