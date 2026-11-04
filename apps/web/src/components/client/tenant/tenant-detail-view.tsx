"use client";

import { useAbility } from "@casl/react";
import { Plus } from "lucide-react";
import { useState } from "react";

import { TenantAccountSection } from "@/components/client/billing/tenant-account-section";
import type { CorrectionRequest } from "@/components/client/billing/type";
import {
  canCreateCorrection,
  canReadAccount,
} from "@/components/client/billing/utils";
import { FormAlert } from "@/components/client/form/form-alert";
import {
  canRecordMeter,
  meterAvailable,
} from "@/components/client/meter/utils";
import { canRecordPayment } from "@/components/client/payment/utils";
import { PayLinkFallback } from "@/components/client/payment-link/pay-link-fallback";
import { useCopyPayLink } from "@/components/client/payment-link/use-copy-pay-link";
import {
  canSharePayLink,
  payLinkInvoice,
} from "@/components/client/payment-link/utils";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import { useGetTenantAccount } from "@/lib/api/generated/money/money";
import { useGetTenant } from "@/lib/api/generated/tenants/tenants";
import type { AppAbility } from "@/lib/capabilities";
import { jakartaToday } from "@/lib/format/date";
import { cn } from "@/lib/utils";
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

// "Penyewa dan sewanya": one tenant, their money (tiles, invoices, payments and corrections,
// for a member with invoice:read and payment:read) and a card for each lease the member may see, with "Catat
// pembayaran" (payment:record), "Ubah penyewa" and, for a tenant without an active lease,
// "Sewakan unit", and "Catat meteran" on a lease that expects metered amounts (charge:record),
// each shown from the member's capabilities (getOrgMe). "Salin link bayar" (invoice:share) copies
// a new payment link for the newest open invoice in the account the money section loads.
// The API answers 404 for a tenant that does not exist and 403 for one the member may not see;
// both read "Penyewa tidak ditemukan".
export function TenantDetailView({ orgId, tenantId }: TenantDetailViewProps) {
  const tenant = useGetTenant(orgId, tenantId);
  const ability = useAbility<AppAbility>();
  const permissions = tenantPermissions(ability);
  const showAccount = canReadAccount(ability);
  // The page's one primary action; "Sewakan unit" is an outline button while it shows.
  const showRecordPayment = canRecordPayment(ability);
  const showRecordMeter = canRecordMeter(ability);
  // "Koreksi tagihan …" in "Catat meteran"'s locked view opens the account's correction dialog,
  // so it shows only where that dialog does.
  const canCorrectInvoice = showAccount && canCreateCorrection(ability);
  const [today] = useState(() => jakartaToday(new Date()));
  const [panel, setPanel] = useState<TenantPanelState>({ kind: "closed" });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");
  const [correctionRequest, setCorrectionRequest] =
    useState<CorrectionRequest | null>(null);
  // "Salin link bayar" needs the open invoices: the same query (and cache entry) as the money
  // section, so it adds no request; without invoice:read and payment:read it never runs.
  const canShare = canSharePayLink(ability);
  const account = useGetTenantAccount(orgId, tenantId, {
    query: { enabled: showAccount && canShare },
  });
  const shareInvoice = payLinkInvoice(canShare && showAccount, account.data);
  const payLink = useCopyPayLink(orgId, setNotice);

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
        canRecordPayment={showRecordPayment}
        onRecordPayment={() =>
          openPanel({
            kind: "payment",
            tenant: data,
            requestId: crypto.randomUUID(),
          })
        }
        copyPayLink={
          shareInvoice === null
            ? null
            : {
                busy: payLink.busy,
                onCopy: () => payLink.copy(shareInvoice.id),
              }
        }
      />
      {payLink.error !== null && <FormAlert message={payLink.error} />}
      {payLink.fallbackUrl !== null && (
        <PayLinkFallback
          url={payLink.fallbackUrl}
          onCopyAgain={payLink.copyAgain}
          onClose={payLink.close}
        />
      )}
      <p
        role="status"
        aria-live="polite"
        className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
      >
        {notice}
      </p>
      {showAccount && (
        <TenantAccountSection
          orgId={orgId}
          tenantId={tenantId}
          tenantName={data.name}
          today={today}
          onNotice={setNotice}
          correctionRequest={correctionRequest}
        />
      )}
      {data.leases.length === 0 ? (
        <NoLeaseCard
          name={data.name}
          canManageLeases={permissions.canManageLeases}
          secondary={showRecordPayment}
          onRent={rent}
        />
      ) : (
        <>
          {permissions.canManageLeases && needsLease(data) && (
            <Button
              type="button"
              variant={showRecordPayment ? "outline" : "default"}
              onClick={rent}
              className={cn(
                "h-12 w-full rounded-xl px-5 text-[17px] font-semibold md:w-auto md:self-start",
                showRecordPayment && "border-[1.5px] bg-card",
              )}
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
              onRecordMeter={
                showRecordMeter && meterAvailable(lease, today)
                  ? () =>
                      openPanel({
                        kind: "meter",
                        lease,
                        tenantName: data.name,
                        opening: crypto.randomUUID(),
                      })
                  : null
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
        onCorrectInvoice={
          canCorrectInvoice
            ? (invoiceId) => {
                setNotice("");
                setCorrectionRequest((previous) => ({
                  id: (previous?.id ?? 0) + 1,
                  invoiceId,
                }));
              }
            : null
        }
      />
    </div>
  );
}
