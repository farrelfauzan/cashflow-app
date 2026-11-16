"use client";

import { useAbility } from "@casl/react";
import { Camera } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import type { AppAbility } from "@/lib/capabilities";

import type { FieldProofTenant } from "./type";
import { FIELD_PROOF_BUTTON_TEXT, canSendFieldProofs } from "./utils";

import { FieldProofSheet } from "./field-proof-sheet";

type SendFieldProofButtonProps = {
  orgId: string;
  // The Tagihan hari ini row's item (a whole FieldTodayItem fits).
  tenant: FieldProofTenant;
  // The row's place line, shown again under the sheet's title.
  placeLabel: string;
};

// "Penyewa sudah transfer? Kirim bukti" under a Tagihan hari ini row's buttons (board
// HP-Bukti-Karyawan, 1): a text button, not a primary, shown only to a member with payment:proof
// (conditional counts; the API decides per tenant). Each press mounts the sheet afresh. The sheet
// stays mounted after it closes, so it can animate out; the next opening replaces it.
export function SendFieldProofButton({
  orgId,
  tenant,
  placeLabel,
}: SendFieldProofButtonProps) {
  const ability = useAbility<AppAbility>();
  const [opening, setOpening] = useState(0);
  const [open, setOpen] = useState(false);

  return (
    <>
      {canSendFieldProofs(ability) && (
        <div className="-mb-1.5 border-t pt-1">
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              setOpening((previous) => previous + 1);
              setOpen(true);
            }}
            className="h-11 w-full gap-2 rounded-xl px-3 text-[15px] font-semibold text-primary hover:text-primary md:text-base"
          >
            <Camera aria-hidden="true" className="size-[18px]" />
            {FIELD_PROOF_BUTTON_TEXT}
          </Button>
        </div>
      )}
      {/* Rendered whatever the ability says now, so a sheet already open is never cut off. */}
      {opening > 0 && (
        <FieldProofSheet
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
