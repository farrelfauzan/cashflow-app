"use client";

import { useAbility } from "@casl/react";
import { Plus } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import { useGetTenant } from "@/lib/api/generated/tenants/tenants";
import type { AppAbility } from "@/lib/capabilities";
import { jakartaToday } from "@/lib/format/date";
import type { TenantPanelState } from "./type";
import {
  isNotFound,
  leaseActions,
  leaseView,
  LOADING_TEXT,
  needsLease,
  orderedLeases,
  tenantPermissions,
} from "./utils";

import { LeaseCard } from "./lease-card";
import { NoLeaseCard } from "./no-lease-card";
import { TenantDetailHeader } from "./tenant-detail-header";
import { TenantLoadError } from "./tenant-load-error";
import { TenantNotFound } from "./tenant-not-found";
import { TenantPanel } from "./tenant-panel";

type TenantDetailViewProps = {
  orgId: string;
  tenantId: string;
};

// "Penyewa dan sewanya": one tenant and a card for each lease the member may see, with "Ubah
// penyewa" and, for a tenant without an active lease, "Sewakan unit", each shown from the
// member's capabilities (getOrgMe). The API answers 404 for a tenant that does not exist and 403
// for one the member may not see; both read "Penyewa tidak ditemukan".
export function TenantDetailView({ orgId, tenantId }: TenantDetailViewProps) {
  const tenant = useGetTenant(orgId, tenantId);
  const permissions = tenantPermissions(useAbility<AppAbility>());
  const [today] = useState(() => jakartaToday(new Date()));
  const [panel, setPanel] = useState<TenantPanelState>({ kind: "closed" });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");

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

  const data = tenant.data;
  function openPanel(next: TenantPanelState) {
    setNotice("");
    setPanel(next);
  }
  const rent = () => openPanel({ kind: "lease", tenant: data });

  return (
    <div className="flex flex-col gap-3.5 md:gap-[22px]">
      <TenantDetailHeader
        orgId={orgId}
        name={data.name}
        phone={data.phone}
        canEdit={permissions.canEditTenant}
        onEdit={() => openPanel({ kind: "edit", tenant: data })}
      />
      <p
        role="status"
        aria-live="polite"
        className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
      >
        {notice}
      </p>
      {data.leases.length === 0 ? (
        <NoLeaseCard
          name={data.name}
          canManageLeases={permissions.canManageLeases}
          onRent={rent}
        />
      ) : (
        <>
          {permissions.canManageLeases && needsLease(data) && (
            <Button
              type="button"
              onClick={rent}
              className="h-12 w-full rounded-xl px-5 text-[17px] font-semibold md:w-auto md:self-start"
            >
              <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
              Sewakan unit
            </Button>
          )}
          {orderedLeases(data.leases, today).map((lease) => (
            <LeaseCard
              key={lease.id}
              lease={leaseView(lease, today)}
              actions={leaseActions(lease, today, permissions.canManageLeases)}
              onAction={(kind) =>
                openPanel({ kind, lease, tenantName: data.name })
              }
            />
          ))}
        </>
      )}
      <TenantPanel
        orgId={orgId}
        organizationName=""
        today={today}
        canManageLeases={permissions.canManageLeases}
        canAddChargeType={permissions.canAddChargeType}
        panel={panel}
        onPanelChange={setPanel}
        onNotice={setNotice}
      />
    </div>
  );
}
