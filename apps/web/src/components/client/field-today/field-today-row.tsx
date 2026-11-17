"use client";

import { ReceiveCashButton } from "@/components/client/field-cash/receive-cash-button";
import { SendFieldProofButton } from "@/components/client/field-proof/send-field-proof-button";
import { RecordPromiseButton } from "@/components/client/field-promise/record-promise-button";
import { RetakePhotoButton } from "@/components/client/receipt-retake/retake-photo-button";
import { TenantChip } from "@/components/client/tenant/tenant-chip";

import type { FieldTodayRowView } from "./type";

type FieldTodayRowProps = {
  orgId: string;
  view: FieldTodayRowView;
};

// One tenant to collect from (board HP-Tagihan, column 1): name and units, why they are on the
// list, what they still owe, the cash taken from them today with its photo ("Ambil ulang" under it
// for each payment whose photo the scan refused, decision 46.10), and "Terima tunai"
// while they still owe, with "Catat janji bayar" beside it and "Penyewa sudah transfer? Kirim
// bukti" under both (board HP-Bukti-Karyawan, 1).
export function FieldTodayRow({ orgId, view }: FieldTodayRowProps) {
  return (
    <li className="flex flex-col gap-2 rounded-[14px] border bg-card px-3.5 py-3 md:px-4 md:py-3.5">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <span className="text-base font-semibold md:text-[17px]">
            {view.tenantName}
          </span>
          {view.place !== "" && (
            <span className="text-sm text-muted-foreground md:text-base">
              {view.place}
            </span>
          )}
        </div>
        <TenantChip tone={view.reason.tone}>{view.reason.label}</TenantChip>
      </div>
      <span className="text-[15px] font-semibold tabular-nums md:text-base">
        {view.owed}
      </span>
      {view.cash !== null && (
        <div className="flex flex-col gap-1 border-t pt-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm text-muted-foreground tabular-nums md:text-base">
              {view.cash.text}
            </span>
            <TenantChip tone={view.cash.photo.tone}>
              {view.cash.photo.label}
            </TenantChip>
          </div>
          {view.cashPayments.map((payment) => (
            <RetakePhotoButton
              key={payment.paymentId}
              orgId={orgId}
              payment={payment}
              withSummary
            />
          ))}
        </div>
      )}
      {view.canCollect && (
        <div className="flex gap-2">
          <ReceiveCashButton
            orgId={orgId}
            tenant={view.item}
            placeLabel={view.place}
            className="flex-1"
          />
          <RecordPromiseButton
            orgId={orgId}
            tenant={view.item}
            placeLabel={view.place}
            className="flex-1"
          />
        </div>
      )}
      {view.canCollect && (
        <SendFieldProofButton
          orgId={orgId}
          tenant={view.item}
          placeLabel={view.place}
        />
      )}
    </li>
  );
}
