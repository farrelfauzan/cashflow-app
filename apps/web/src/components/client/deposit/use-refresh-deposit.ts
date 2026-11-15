"use client";

import { useQueryClient } from "@tanstack/react-query";

import {
  getGetMyFieldCashQueryKey,
  getListMyCashDepositsQueryKey,
  getListMyUndepositedCashQueryKey,
} from "@/lib/api/generated/field/field";
import type { CashTotal } from "@/lib/api/generated/model";

// After a deposit, or when the API says the cash held changed: reload the total, the receipts
// and the deposits (every page). Cached money is never patched; the API's answer is the one
// shown. Resolves, once the active queries have refetched, with the total the API answers now.
export function useRefreshDeposit(
  orgId: string,
): () => Promise<CashTotal | undefined> {
  const queryClient = useQueryClient();
  return async () => {
    const cashKey = getGetMyFieldCashQueryKey(orgId);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: cashKey }),
      queryClient.invalidateQueries({
        queryKey: getListMyUndepositedCashQueryKey(orgId),
      }),
      queryClient.invalidateQueries({
        queryKey: getListMyCashDepositsQueryKey(orgId),
      }),
    ]);
    return queryClient.getQueryData<CashTotal>(cashKey);
  };
}
