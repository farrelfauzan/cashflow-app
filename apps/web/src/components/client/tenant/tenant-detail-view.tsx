"use client";

import { useState } from "react";

import { toFormError } from "@/lib/api/form-error";
import { useGetTenant } from "@/lib/api/generated/tenants/tenants";
import { jakartaToday } from "@/lib/format/date";
import { isNotFound, leaseView, LOADING_TEXT, orderedLeases } from "./utils";

import { LeaseCard } from "./lease-card";
import { NoLeaseCard } from "./no-lease-card";
import { TenantDetailHeader } from "./tenant-detail-header";
import { TenantLoadError } from "./tenant-load-error";
import { TenantNotFound } from "./tenant-not-found";

type TenantDetailViewProps = {
  orgId: string;
  tenantId: string;
};

// "Penyewa dan sewanya": one tenant and a card for each lease the member may see, read-only.
// The API answers 404 for a tenant that does not exist and 403 for one the member may not see;
// both read "Penyewa tidak ditemukan".
export function TenantDetailView({ orgId, tenantId }: TenantDetailViewProps) {
  const tenant = useGetTenant(orgId, tenantId);
  const [today] = useState(() => jakartaToday(new Date()));

  if (tenant.isError) {
    if (isNotFound(tenant.error)) return <TenantNotFound orgId={orgId} />;
    return (
      <TenantLoadError
        message={toFormError(tenant.error, []).message}
        onRetry={() => void tenant.refetch()}
      />
    );
  }
  if (tenant.data === undefined) {
    return <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>;
  }

  return (
    <div className="flex flex-col gap-3.5 md:gap-[22px]">
      <TenantDetailHeader
        orgId={orgId}
        name={tenant.data.name}
        phone={tenant.data.phone}
      />
      {tenant.data.leases.length === 0 ? (
        <NoLeaseCard />
      ) : (
        orderedLeases(tenant.data.leases, today).map((lease) => (
          <LeaseCard key={lease.id} lease={leaseView(lease, today)} />
        ))
      )}
    </div>
  );
}
