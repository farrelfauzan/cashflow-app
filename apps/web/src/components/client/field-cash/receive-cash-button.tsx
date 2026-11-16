"use client";

import { useAbility } from "@casl/react";
import { cn } from "cn";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import type { AppAbility } from "@/lib/capabilities";

import type { ReceiveCashTenant } from "./type";
import { RECEIVE_CASH_BUTTON_TEXT, canRecordCash } from "./utils";

import { ReceiveCashSheet } from "./receive-cash-sheet";

type ReceiveCashButtonProps = {
  orgId: string;
  // The Tagihan hari ini row's item (a whole FieldTodayItem fits).
  tenant: ReceiveCashTenant;
  // The row's place line ("Kos Melati 11"), shown again under the sheet's title.
  placeLabel: string;
  // Sizing from the row ("flex-1").
  className?: string;
};

// "Terima tunai" on a Tagihan hari ini row: the row's primary, shown only to a member with
// cash:record (conditional counts; the API decides per tenant). Each press opens the sheet afresh
// with a new requestId, kept for every retry of that opening, except after an opening left unsure
// whether the cash was recorded: that one is reopened as it was. The sheet stays mounted after it
// closes, so it can animate out; the next opening replaces it.
export function ReceiveCashButton({
  orgId,
  tenant,
  placeLabel,
  className,
}: ReceiveCashButtonProps) {
  const ability = useAbility<AppAbility>();
  const [requestId, setRequestId] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  // The last opening's save may have reached the API with its answer lost ("belum pasti
  // tersimpan"). Pressing Terima tunai again then reopens that opening, with its amount, photo
  // and requestId, instead of starting a new one that would record the same cash twice.
  const [unsure, setUnsure] = useState(false);

  return (
    <>
      {canRecordCash(ability) && (
        <Button
          type="button"
          onClick={() => {
            if (!unsure || requestId === null) {
              setRequestId(crypto.randomUUID());
            }
            setOpen(true);
          }}
          className={cn(
            "h-12 rounded-xl px-4 text-base font-semibold",
            className,
          )}
        >
          {RECEIVE_CASH_BUTTON_TEXT}
        </Button>
      )}
      {/* Rendered whatever the ability says now, so a sheet already open is never cut off. */}
      {requestId !== null && (
        <ReceiveCashSheet
          key={requestId}
          orgId={orgId}
          tenant={tenant}
          placeLabel={placeLabel}
          requestId={requestId}
          open={open}
          onOpenChange={setOpen}
          onUnsureChange={setUnsure}
        />
      )}
    </>
  );
}
