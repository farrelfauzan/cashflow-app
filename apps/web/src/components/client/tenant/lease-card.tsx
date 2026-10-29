"use client";

import { Button } from "@/components/ui/button";
import type { LeaseActions, LeasePanelKind, LeaseView } from "./type";
import {
  LEASE_ACTION_CLASS,
  LEASE_PANEL_TITLES,
  leaseFooterKinds,
} from "./utils";

import { LeaseCharges } from "./lease-charges";
import { LeaseField } from "./lease-field";
import { TenantChip } from "./tenant-chip";

type LeaseCardProps = {
  lease: LeaseView;
  // The buttons that change the lease; null shows none (an ended lease, or no lease:manage).
  actions: LeaseActions | null;
  onAction: (kind: LeasePanelKind) => void;
};

// One lease of the tenant (boards Penyewa-Sewa.dc.html and Penyewa-Sewa-Phone.dc.html): the unit
// and its status, the terms, who collects, and the dates. On an active lease a member with
// lease:manage gets "Ubah sewa" (in the header on iPad, above the card on the phone) and a footer
// with "Biaya tambahan", "Rencana keluar" and "Catat pindah keluar", or "Batalkan sewa" for a
// booking.
export function LeaseCard({ lease, actions, onAction }: LeaseCardProps) {
  return (
    <>
      {actions !== null && (
        <Button
          type="button"
          onClick={() => onAction("editLease")}
          className="h-13 w-full rounded-xl px-5 text-[17px] font-semibold md:hidden"
        >
          {LEASE_PANEL_TITLES.editLease}
        </Button>
      )}
      <section
        aria-labelledby={`lease-${lease.id}`}
        className="flex flex-col gap-1 rounded-[14px] border bg-card p-4 md:gap-5 md:rounded-2xl md:px-6 md:py-[22px]"
      >
        <div className="flex items-center justify-between gap-4 pb-2.5 md:pb-0">
          <div className="flex flex-wrap items-center gap-2.5 md:gap-3">
            <h2
              id={`lease-${lease.id}`}
              className="text-xl font-bold md:text-2xl"
            >
              {lease.title}
            </h2>
            <TenantChip tone={lease.status.tone}>
              {lease.status.label}
            </TenantChip>
          </div>
          {actions !== null && (
            <Button
              type="button"
              onClick={() => onAction("editLease")}
              className="hidden h-12 shrink-0 rounded-xl px-5 text-[17px] font-semibold md:inline-flex"
            >
              {LEASE_PANEL_TITLES.editLease}
            </Button>
          )}
        </div>

        <div className="flex flex-col md:grid md:grid-cols-3 md:gap-x-6 md:gap-y-5">
          <LeaseField label="Sewa per bulan">{lease.rent}</LeaseField>
          <LeaseField label="Uang jaminan">{lease.deposit}</LeaseField>
          <LeaseField label="Penagih">
            {lease.collector === null ? (
              <span className="font-normal text-muted-foreground">
                Tidak ada
              </span>
            ) : (
              <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                {lease.collector.name}
                {lease.collector.inactive && (
                  <TenantChip tone="warning">
                    Nonaktif, belum diganti
                  </TenantChip>
                )}
              </span>
            )}
          </LeaseField>
          <LeaseField label="Mulai sewa">{lease.startDate}</LeaseField>
          <LeaseField label="Mulai tagih dari">
            {lease.billedFrom}
            {lease.billedNote !== null && (
              <div className="text-sm leading-[1.4] font-normal text-muted-foreground md:text-base">
                {lease.billedNote}
              </div>
            )}
          </LeaseField>
          {lease.plannedEnd !== null && (
            <LeaseField label="Rencana keluar">
              {lease.plannedEnd.set ? (
                lease.plannedEnd.text
              ) : (
                <span className="font-normal text-muted-foreground">
                  {lease.plannedEnd.text}
                </span>
              )}
            </LeaseField>
          )}
          {lease.endDate !== null && (
            <LeaseField label="Keluar">{lease.endDate}</LeaseField>
          )}
        </div>

        <LeaseCharges charges={lease.charges} />

        <p className="pt-2 text-sm text-muted-foreground md:pt-0 md:text-base">
          {lease.entered}
        </p>

        {actions !== null && (
          <div className="mt-3 flex flex-col gap-2.5 border-t pt-4 md:mt-0 md:flex-row">
            {leaseFooterKinds(actions).map((kind) => (
              <Button
                key={kind}
                type="button"
                variant="outline"
                onClick={() => onAction(kind)}
                className={LEASE_ACTION_CLASS}
              >
                {LEASE_PANEL_TITLES[kind]}
              </Button>
            ))}
          </div>
        )}
      </section>
    </>
  );
}
