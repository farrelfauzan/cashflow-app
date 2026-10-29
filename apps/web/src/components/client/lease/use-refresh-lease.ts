"use client";

import { useQueryClient, type QueryKey } from "@tanstack/react-query";

import { getListMembersQueryKey } from "@/lib/api/generated/members/members";
import type { Lease } from "@/lib/api/generated/model";
import {
  getGetPropertyQueryKey,
  getListPropertiesQueryKey,
  getListUnitsQueryKey,
} from "@/lib/api/generated/properties/properties";
import {
  getGetTenantQueryKey,
  getListTenantsQueryKey,
} from "@/lib/api/generated/tenants/tenants";

// After a lease changes: reload its tenant, the tenant list and the member list (whose count of
// leases per collector feeds "penyewa belum punya penagih"), and when the unit is free again
// (a move-out or a cancelled booking) the unit list and the property's occupancy too. Cached
// values are never patched; the API's answer is the one shown.
export function useRefreshLease(
  orgId: string,
): (lease: Lease, unitFreed: boolean) => Promise<void> {
  const queryClient = useQueryClient();
  return async (lease, unitFreed) => {
    const keys: QueryKey[] = [
      getGetTenantQueryKey(orgId, lease.tenantId),
      getListTenantsQueryKey(orgId),
      getListMembersQueryKey(orgId),
    ];
    if (unitFreed) {
      keys.push(
        getListUnitsQueryKey(orgId, lease.propertyId),
        getListPropertiesQueryKey(orgId),
        getGetPropertyQueryKey(orgId, lease.propertyId),
      );
    }
    await Promise.all(
      keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
  };
}
