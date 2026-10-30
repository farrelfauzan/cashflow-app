"use client";

import type { UnitPriceTermView } from "./type";

type UnitPriceTermRowProps = {
  term: UnitPriceTermView;
};

// One offered term in "Harga unit": its name and price in force, and under them the price still to
// come.
export function UnitPriceTermRow({ term }: UnitPriceTermRowProps) {
  return (
    <li className="flex flex-col gap-1 border-t py-3 first:border-t-0">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-base font-semibold md:text-[17px]">
          {term.label}
        </span>
        <span className="text-base font-semibold md:text-[17px]">
          {term.price}
        </span>
      </div>
      {term.next !== null && (
        <span className="text-sm leading-[1.45] text-muted-foreground md:text-base">
          {term.next}
        </span>
      )}
    </li>
  );
}
