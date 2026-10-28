"use client";

import type { LeaseView } from "./type";

import { LeaseCharges } from "./lease-charges";
import { LeaseField } from "./lease-field";
import { TenantChip } from "./tenant-chip";

type LeaseCardProps = {
  lease: LeaseView;
};

// One lease of the tenant (boards Penyewa-Sewa.dc.html and Penyewa-Sewa-Phone.dc.html), read-only:
// the unit and its status, the terms, who collects, and the dates.
export function LeaseCard({ lease }: LeaseCardProps) {
  return (
    <section
      aria-labelledby={`lease-${lease.id}`}
      className="flex flex-col gap-1 rounded-[14px] border bg-card p-4 md:gap-5 md:rounded-2xl md:px-6 md:py-[22px]"
    >
      <div className="flex flex-wrap items-center gap-2.5 pb-2.5 md:gap-3 md:pb-0">
        <h2 id={`lease-${lease.id}`} className="text-xl font-bold md:text-2xl">
          {lease.title}
        </h2>
        <TenantChip tone={lease.status.tone}>{lease.status.label}</TenantChip>
      </div>

      <div className="flex flex-col md:grid md:grid-cols-3 md:gap-x-6 md:gap-y-5">
        <LeaseField label="Sewa per bulan">{lease.rent}</LeaseField>
        <LeaseField label="Uang jaminan">{lease.deposit}</LeaseField>
        <LeaseField label="Penagih">
          {lease.collector === null ? (
            <span className="font-normal text-muted-foreground">Tidak ada</span>
          ) : (
            <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
              {lease.collector.name}
              {lease.collector.inactive && (
                <TenantChip tone="warning">Nonaktif, belum diganti</TenantChip>
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
    </section>
  );
}
