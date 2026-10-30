"use client";

import { useId } from "react";

import { Button } from "@/components/ui/button";
import { useListUnitPrices } from "@/lib/api/generated/properties/properties";
import {
  LOADING_TEXT,
  NO_PRICE_HISTORY_TEXT,
  PRICE_HISTORY_ERROR_TEXT,
  PRICE_HISTORY_TITLE,
  priceHistoryRows,
} from "./utils";

import { UnitPriceHistoryRow } from "./unit-price-history-row";

type UnitPriceHistoryProps = {
  orgId: string;
  unitId: string;
  // "Today" in Jakarta, read once when the panel opened.
  today: string;
};

// "Riwayat harga": every price recorded for the unit, as the API orders them.
export function UnitPriceHistory({
  orgId,
  unitId,
  today,
}: UnitPriceHistoryProps) {
  const id = useId();
  const history = useListUnitPrices(orgId, unitId);

  return (
    <section aria-labelledby={`${id}-title`} className="flex flex-col gap-2">
      <h3
        id={`${id}-title`}
        className="text-[15px] leading-normal font-semibold md:text-base"
      >
        {PRICE_HISTORY_TITLE}
      </h3>
      {history.isError ? (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-[15px] text-foreground md:text-base">
            {PRICE_HISTORY_ERROR_TEXT}
          </p>
          <Button
            type="button"
            variant="outline"
            onClick={() => void history.refetch()}
            className="h-11 rounded-xl border-[1.5px] bg-card px-4 text-[15px] font-semibold md:text-base"
          >
            Coba lagi
          </Button>
        </div>
      ) : history.data === undefined ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {LOADING_TEXT}
        </p>
      ) : history.data.length === 0 ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {NO_PRICE_HISTORY_TEXT}
        </p>
      ) : (
        <ul
          aria-label={PRICE_HISTORY_TITLE}
          className="flex flex-col rounded-xl border-[1.5px] bg-card px-3.5"
        >
          {priceHistoryRows(history.data, today).map((row) => (
            <UnitPriceHistoryRow key={row.id} row={row} />
          ))}
        </ul>
      )}
    </section>
  );
}
