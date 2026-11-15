"use client";

import { useQueryClient } from "@tanstack/react-query";

import { getListCheckQueueQueryKey } from "@/lib/api/generated/check-queue/check-queue";
import { getListCashDepositsQueryKey } from "@/lib/api/generated/money/money";

// After a count, or when the API says the deposit was already counted: reload every list of
// deposits (both tabs, every page, and Penagihan's count), and Antrean cek (every page and kind,
// and Penagihan's count), where a count with a gap adds an item. Cached money is never patched;
// the API's answer is the one shown.
export function useRefreshDeposits(orgId: string): () => Promise<void> {
  const queryClient = useQueryClient();
  return async () => {
    await Promise.all(
      [
        getListCashDepositsQueryKey(orgId),
        getListCheckQueueQueryKey(orgId),
      ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
  };
}
