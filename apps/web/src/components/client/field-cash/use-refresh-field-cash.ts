"use client";

import { useQueryClient } from "@tanstack/react-query";

import {
  getGetMyFieldCashQueryKey,
  getListFieldTodayQueryKey,
  getListMyUndepositedCashQueryKey,
} from "@/lib/api/generated/field/field";

// After cash is recorded (or may have been, when an answer was lost): reload Tagihan hari ini
// (every page), "Tunai di tangan" and the receipts held for Setor tunai (every page). The keys
// are built without params, so they match every variant. Cached money is never patched; the
// API's answer is the one shown.
export function useRefreshFieldCash(orgId: string): () => Promise<void> {
  const queryClient = useQueryClient();
  return async () => {
    await Promise.all(
      [
        getListFieldTodayQueryKey(orgId),
        getGetMyFieldCashQueryKey(orgId),
        getListMyUndepositedCashQueryKey(orgId),
      ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
  };
}
