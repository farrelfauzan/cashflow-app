"use client";

import { useQueryClient } from "@tanstack/react-query";

import {
  getGetDashboardQueryKey,
  getGetTenantAccountQueryKey,
  getListTenantOpenInvoicesQueryKey,
} from "@/lib/api/generated/money/money";
import { isInvoicesQueryKey } from "./utils";

// After a payment is recorded: reload the tenant's account (the page behind the panel) and open
// invoices ("Salin link bayar"), the dashboard and every invoice list. Cached money is never
// patched; the API's answer is the one shown. The promise settles once the active queries have
// refetched.
export function useRefreshPayment(
  orgId: string,
  tenantId: string,
): () => Promise<void> {
  const queryClient = useQueryClient();
  return async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: getGetTenantAccountQueryKey(orgId, tenantId),
      }),
      queryClient.invalidateQueries({
        queryKey: getListTenantOpenInvoicesQueryKey(orgId, tenantId),
      }),
      queryClient.invalidateQueries({
        queryKey: getGetDashboardQueryKey(orgId),
      }),
      queryClient.invalidateQueries({
        predicate: (query) => isInvoicesQueryKey(query.queryKey, orgId),
      }),
    ]);
  };
}
