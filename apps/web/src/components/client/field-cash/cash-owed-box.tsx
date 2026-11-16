"use client";

import { formatRupiah } from "@/lib/format/rupiah";

import type { ReceiveCashTenant } from "./type";
import { OWED_LABEL, owedLine } from "./utils";

type CashOwedBoxProps = {
  tenant: ReceiveCashTenant;
};

// The top of "Terima tunai": what the tenant owes over all their invoices and when the oldest
// was due, both from their Tagihan hari ini row.
export function CashOwedBox({ tenant }: CashOwedBoxProps) {
  const line = owedLine(tenant);
  return (
    <section
      aria-label={OWED_LABEL}
      className="flex flex-col gap-1 rounded-xl bg-muted px-3.5 py-3"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <span className="text-[15px] font-semibold md:text-base">
          {OWED_LABEL}
        </span>
        <span className="text-xl font-bold tabular-nums">
          {formatRupiah(tenant.outstanding)}
        </span>
      </div>
      {line !== null && (
        <p className="text-sm text-muted-foreground md:text-base">{line}</p>
      )}
    </section>
  );
}
