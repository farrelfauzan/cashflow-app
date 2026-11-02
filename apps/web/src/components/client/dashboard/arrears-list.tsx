"use client";

import type { ArrearsRowView } from "./type";
import { ARREARS_LIST_LABEL, ARREARS_TITLE, NO_ARREARS_TEXT } from "./utils";

import { ArrearsRow } from "./arrears-row";

type ArrearsListProps = {
  orgId: string;
  // "9 orang": every tenant in arrears.
  count: string;
  rows: readonly ArrearsRowView[];
  // "dan 3 penyewa lain" when the API listed only the first ten.
  more: string | null;
  canOpenTenant: boolean;
};

// "Belum bayar": the tenants who owe money after its due date, the oldest debt first.
export function ArrearsList({
  orgId,
  count,
  rows,
  more,
  canOpenTenant,
}: ArrearsListProps) {
  return (
    <section className="flex min-w-0 flex-col gap-1 rounded-[14px] border bg-card p-4 md:rounded-2xl md:px-6 md:py-[22px]">
      <h2 className="text-xl font-bold md:text-2xl">
        {ARREARS_TITLE}{" "}
        <span className="font-normal text-muted-foreground">· {count}</span>
      </h2>
      {rows.length === 0 ? (
        <p className="pt-2 text-[15px] text-muted-foreground md:text-base">
          {NO_ARREARS_TEXT}
        </p>
      ) : (
        <ul aria-label={ARREARS_LIST_LABEL} className="flex flex-col">
          {rows.map((row) => (
            <ArrearsRow
              key={row.tenantId}
              orgId={orgId}
              row={row}
              canOpenTenant={canOpenTenant}
            />
          ))}
        </ul>
      )}
      {more !== null && (
        <p className="border-t pt-3 text-[15px] text-muted-foreground md:text-base">
          {more}
        </p>
      )}
    </section>
  );
}
