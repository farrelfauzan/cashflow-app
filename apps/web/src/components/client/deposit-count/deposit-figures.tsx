"use client";

import type { CountSheetView } from "./type";
import { EXPECTED_LABEL, HANDED_LABEL } from "./utils";

type DepositFiguresProps = {
  view: CountSheetView;
};

// The two numbers side by side: the receipts' total and what the karyawan says they handed over,
// with the photo of the money as text.
export function DepositFigures({ view }: DepositFiguresProps) {
  return (
    <dl className="grid grid-cols-2 gap-2.5">
      <div className="flex flex-col gap-0.5 rounded-xl bg-muted px-3.5 py-3">
        <dt className="text-sm text-muted-foreground md:text-base">
          {EXPECTED_LABEL}
        </dt>
        <dd className="text-xl font-bold tabular-nums">{view.expected}</dd>
        <dd className="text-sm text-muted-foreground md:text-base">
          {view.receipts}
        </dd>
      </div>
      <div className="flex flex-col gap-0.5 rounded-xl bg-muted px-3.5 py-3">
        <dt className="text-sm text-muted-foreground md:text-base">
          {HANDED_LABEL}
        </dt>
        <dd className="text-xl font-bold tabular-nums">{view.handed}</dd>
        <dd className="text-sm text-muted-foreground md:text-base">
          {view.photo}
        </dd>
      </div>
    </dl>
  );
}
