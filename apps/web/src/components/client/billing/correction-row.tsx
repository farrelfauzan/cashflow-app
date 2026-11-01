"use client";

import type { CorrectionRowView } from "./type";

type CorrectionRowProps = {
  correction: CorrectionRowView;
};

// One correction: when, of what, the signed amount, its reason and who made it.
export function CorrectionRow({ correction }: CorrectionRowProps) {
  return (
    <li className="flex flex-col gap-1 border-t pt-3 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-1">
        <h3 className="text-base font-bold md:text-[17px]">
          {correction.title}
        </h3>
        <span className="text-base font-bold md:text-[17px]">
          {correction.amount}
        </span>
      </div>
      <p className="text-[15px] md:text-base">{correction.reason}</p>
      <span className="text-sm text-muted-foreground md:text-base">
        {correction.by}
      </span>
    </li>
  );
}
