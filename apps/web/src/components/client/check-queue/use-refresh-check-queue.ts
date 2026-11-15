"use client";

import { useQueryClient } from "@tanstack/react-query";

import { getListCheckQueueQueryKey } from "@/lib/api/generated/check-queue/check-queue";
import { getListCashDepositsQueryKey } from "@/lib/api/generated/money/money";
import {
  getGetTenantQueryKey,
  getListTenantsQueryKey,
} from "@/lib/api/generated/tenants/tenants";

import type { CheckRefreshScope } from "./type";

// After an item is answered (by this member or, says the API, by another): reload every page
// and kind of the queue, and whatever else the answer changed. Cached values are never patched;
// the API's answer is the one shown.
export function useRefreshCheckQueue(
  orgId: string,
): (scope?: CheckRefreshScope) => Promise<void> {
  const queryClient = useQueryClient();
  return async (scope = {}) => {
    const keys: (readonly unknown[])[] = [getListCheckQueueQueryKey(orgId)];
    if (scope.deposits === true) keys.push(getListCashDepositsQueryKey(orgId));
    if (scope.tenantId !== undefined) {
      keys.push(getGetTenantQueryKey(orgId, scope.tenantId));
      keys.push(getListTenantsQueryKey(orgId));
    }
    await Promise.all(
      keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
  };
}
