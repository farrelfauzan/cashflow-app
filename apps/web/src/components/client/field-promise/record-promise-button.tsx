"use client";

import { useAbility } from "@casl/react";
import { cn } from "cn";
import { useState } from "react";

import { canCreatePromise } from "@/components/client/billing/utils";
import { Button } from "@/components/ui/button";
import type { AppAbility } from "@/lib/capabilities";

import type { FieldPromiseTenant } from "./type";
import { RECORD_FIELD_PROMISE_TEXT } from "./utils";

import { FieldPromiseSheet } from "./field-promise-sheet";

type RecordPromiseButtonProps = {
  orgId: string;
  // The Tagihan hari ini row's item (a whole FieldTodayItem fits).
  tenant: FieldPromiseTenant;
  // The row's place line, shown again under the sheet's title.
  placeLabel: string;
  // Sizing from the row ("flex-1").
  className?: string;
};

// "Catat janji bayar" on a Tagihan hari ini row: an outline button beside the primary "Terima
// tunai", shown only to a member with promise:create (conditional counts; the API decides per
// tenant). Each press mounts the sheet afresh, so it reads the offer again. The sheet stays
// mounted after it closes, so it can animate out; the next opening replaces it.
export function RecordPromiseButton({
  orgId,
  tenant,
  placeLabel,
  className,
}: RecordPromiseButtonProps) {
  const ability = useAbility<AppAbility>();
  const [opening, setOpening] = useState(0);
  const [open, setOpen] = useState(false);

  return (
    <>
      {canCreatePromise(ability) && (
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            setOpening((previous) => previous + 1);
            setOpen(true);
          }}
          className={cn(
            "h-12 rounded-xl border-[1.5px] bg-card px-4 text-base font-semibold",
            className,
          )}
        >
          {RECORD_FIELD_PROMISE_TEXT}
        </Button>
      )}
      {/* Rendered whatever the ability says now, so a sheet already open is never cut off. */}
      {opening > 0 && (
        <FieldPromiseSheet
          key={opening}
          orgId={orgId}
          tenant={tenant}
          placeLabel={placeLabel}
          open={open}
          onOpenChange={setOpen}
        />
      )}
    </>
  );
}
