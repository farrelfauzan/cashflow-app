"use client";

import { useIsMutating } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";

import { CancelBookingForm } from "@/components/client/lease/cancel-booking-form";
import { LeaseChargesForm } from "@/components/client/lease/lease-charges-form";
import { LeaseEditForm } from "@/components/client/lease/lease-edit-form";
import { LeaseForm } from "@/components/client/lease/lease-form";
import { MoveOutForm } from "@/components/client/lease/move-out-form";
import { PlannedEndForm } from "@/components/client/lease/planned-end-form";
import { leaseFormSubtitle } from "@/components/client/lease/utils";
import { RecordMeterPanel } from "@/components/client/meter/record-meter-panel";
import { METER_PANEL_TITLE } from "@/components/client/meter/utils";
import { PaymentRecorded } from "@/components/client/payment/payment-recorded";
import { RecordPaymentForm } from "@/components/client/payment/record-payment-form";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  getRecordMeteredChargeMutationKey,
  getRecordPaymentMutationKey,
} from "@/lib/api/generated/money/money";
import { cn } from "@/lib/utils";
import type { OpenTenantPanel, TenantPanelState } from "./type";
import {
  bookingCancelledNotice,
  LEASE_PANEL_TITLES,
  leaseChargesNotice,
  leasePanelSubtitle,
  leaseSavedNotice,
  leaseUpdatedNotice,
  movedOutNotice,
  PAYMENT_PANEL_TITLE,
  paymentPanelSubtitle,
  plannedEndNotice,
  tenantAddedNotice,
  tenantSavedNotice,
} from "./utils";

import { TenantAdded } from "./tenant-added";
import { TenantForm } from "./tenant-form";

type TenantPanelProps = {
  orgId: string;
  // The subtitle while adding a tenant.
  organizationName: string;
  // "Today" in Jakarta, for the lease forms.
  today: string;
  canManageLeases: boolean;
  // "Tambah jenis biaya baru" in "Biaya tambahan".
  canAddChargeType: boolean;
  panel: TenantPanelState;
  onPanelChange: (panel: TenantPanelState) => void;
  onNotice: (notice: string) => void;
  // "Koreksi tagihan …" from "Catat meteran"'s locked view: the panel closes and the page opens
  // the correction dialog for the invoice; null hides it (no correction:create).
  onCorrectInvoice: ((invoiceId: string) => void) | null;
};

// The side panel of the tenant screens (boards Penyewa-Tambah.dc.html and Sewa-Baru.dc.html):
// from the right on iPad, the whole screen on the phone. "Tambah penyewa" goes on to "Penyewa
// ditambahkan" and from there to "Sewakan unit" in the same panel. The lease form gets the wide
// panel, so "Mulai tagih dari" stays in view next to the other fields, and so does "Ubah sewa"
// (Sewa-Ubah.dc.html). "Biaya tambahan", "Rencana keluar", "Catat pindah keluar" and "Batalkan
// sewa" (Sewa-Biaya, Sewa-Rencana, Sewa-Pindah and Sewa-Batal) open from a lease's card in the
// narrow panel. "Catat pembayaran" opens from the header in the narrow panel and shows
// "Pembayaran tercatat" there once saved; the panel cannot be closed while it saves. "Catat
// meteran" opens from a lease's card in the narrow panel and stays open after each save.
export function TenantPanel({
  orgId,
  organizationName,
  today,
  canManageLeases,
  canAddChargeType,
  panel,
  onPanelChange,
  onNotice,
  onCorrectInvoice,
}: TenantPanelProps) {
  // The last panel that was open stays rendered while the sheet slides away.
  const [shown, setShown] = useState<OpenTenantPanel | null>(null);
  if (panel.kind !== "closed" && panel !== shown) setShown(panel);
  const close = () => onPanelChange({ kind: "closed" });
  // A payment being recorded keeps the panel open, so its answer is never lost.
  // So is a meter amount being saved.
  const payments = useIsMutating({
    mutationKey: getRecordPaymentMutationKey(),
  });
  const meters = useIsMutating({
    mutationKey: getRecordMeteredChargeMutationKey(),
  });
  const recording = payments + meters > 0;

  let title = "";
  let subtitle = "";
  let body: ReactNode = null;
  switch (shown?.kind) {
    case "add":
      title = "Tambah penyewa";
      subtitle = organizationName;
      body = (
        <TenantForm
          orgId={orgId}
          tenant={undefined}
          onSaved={(tenant) => onPanelChange({ kind: "added", tenant })}
        />
      );
      break;
    case "added":
      title = "Penyewa ditambahkan";
      subtitle = organizationName;
      body = (
        <TenantAdded
          name={shown.tenant.name}
          canManageLeases={canManageLeases}
          onLater={() => {
            onNotice(tenantAddedNotice(shown.tenant.name));
            close();
          }}
          onRent={() => onPanelChange({ kind: "lease", tenant: shown.tenant })}
        />
      );
      break;
    case "edit":
      title = "Ubah penyewa";
      subtitle = leaseFormSubtitle(shown.tenant.name, shown.tenant.phone);
      body = (
        <TenantForm
          key={shown.tenant.id}
          orgId={orgId}
          tenant={shown.tenant}
          onSaved={(tenant) => {
            onNotice(tenantSavedNotice(tenant.name));
            close();
          }}
        />
      );
      break;
    case "lease":
      title = "Sewakan unit";
      subtitle = leaseFormSubtitle(shown.tenant.name, shown.tenant.phone);
      body = (
        <LeaseForm
          key={shown.tenant.id}
          orgId={orgId}
          tenant={shown.tenant}
          today={today}
          onSaved={(lease) => {
            onNotice(leaseSavedNotice(lease, shown.tenant.name));
            close();
          }}
        />
      );
      break;
    case "editLease":
      title = LEASE_PANEL_TITLES.editLease;
      subtitle = leasePanelSubtitle(shown.tenantName, shown.lease);
      body = (
        <LeaseEditForm
          key={shown.lease.id}
          orgId={orgId}
          lease={shown.lease}
          tenantName={shown.tenantName}
          today={today}
          onSaved={(lease) => {
            onNotice(leaseUpdatedNotice(lease));
            close();
          }}
        />
      );
      break;
    case "charges":
      title = LEASE_PANEL_TITLES.charges;
      subtitle = leasePanelSubtitle(shown.tenantName, shown.lease);
      body = (
        <LeaseChargesForm
          key={shown.lease.id}
          orgId={orgId}
          lease={shown.lease}
          canAddChargeType={canAddChargeType}
          onSaved={(lease) => {
            onNotice(leaseChargesNotice(lease));
            close();
          }}
        />
      );
      break;
    case "plannedEnd":
      title = LEASE_PANEL_TITLES.plannedEnd;
      subtitle = leasePanelSubtitle(shown.tenantName, shown.lease);
      body = (
        <PlannedEndForm
          key={shown.lease.id}
          orgId={orgId}
          lease={shown.lease}
          today={today}
          onSaved={(lease) => {
            onNotice(plannedEndNotice(lease));
            close();
          }}
        />
      );
      break;
    case "moveOut":
      title = LEASE_PANEL_TITLES.moveOut;
      subtitle = leasePanelSubtitle(shown.tenantName, shown.lease);
      body = (
        <MoveOutForm
          key={shown.lease.id}
          orgId={orgId}
          lease={shown.lease}
          today={today}
          onSaved={(lease) => {
            onNotice(movedOutNotice(lease));
            close();
          }}
        />
      );
      break;
    case "cancelBooking":
      title = LEASE_PANEL_TITLES.cancelBooking;
      subtitle = leasePanelSubtitle(shown.tenantName, shown.lease);
      body = (
        <CancelBookingForm
          key={shown.lease.id}
          orgId={orgId}
          lease={shown.lease}
          onSaved={(lease) => {
            onNotice(bookingCancelledNotice(lease));
            close();
          }}
        />
      );
      break;
    case "payment":
      title = PAYMENT_PANEL_TITLE;
      subtitle = paymentPanelSubtitle(shown.tenant, today);
      body = (
        <RecordPaymentForm
          key={shown.requestId}
          orgId={orgId}
          tenantId={shown.tenant.id}
          tenantName={shown.tenant.name}
          today={today}
          requestId={shown.requestId}
          onSaved={(result) =>
            onPanelChange({
              kind: "paymentRecorded",
              tenant: shown.tenant,
              result,
            })
          }
        />
      );
      break;
    case "paymentRecorded":
      title = PAYMENT_PANEL_TITLE;
      subtitle = paymentPanelSubtitle(shown.tenant, today);
      body = <PaymentRecorded result={shown.result} onDone={close} />;
      break;
    case "meter":
      title = METER_PANEL_TITLE;
      subtitle = leasePanelSubtitle(shown.tenantName, shown.lease);
      body = (
        <RecordMeterPanel
          key={shown.opening}
          orgId={orgId}
          lease={shown.lease}
          today={today}
          onCorrectInvoice={
            onCorrectInvoice === null
              ? null
              : (invoiceId) => {
                  close();
                  onCorrectInvoice(invoiceId);
                }
          }
        />
      );
      break;
  }

  return (
    <Sheet
      open={panel.kind !== "closed"}
      onOpenChange={(next) => {
        if (!next && !recording) close();
      }}
    >
      <SheetContent
        side="right"
        className={cn(
          "w-full gap-0 bg-card p-0 pr-[env(safe-area-inset-right)] pb-[env(safe-area-inset-bottom)] sm:max-w-none",
          shown?.kind === "lease" || shown?.kind === "editLease"
            ? "md:w-[920px] md:max-w-full"
            : "md:w-[470px]",
        )}
      >
        <SheetHeader className="gap-1 border-b px-6 pt-6 pr-16 pb-4">
          <SheetTitle className="text-2xl font-bold">{title}</SheetTitle>
          <SheetDescription className="text-base text-muted-foreground">
            {subtitle}
          </SheetDescription>
        </SheetHeader>
        {body}
      </SheetContent>
    </Sheet>
  );
}
