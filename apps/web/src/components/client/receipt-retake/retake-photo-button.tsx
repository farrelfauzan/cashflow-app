"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { canRecordCash } from "@/components/client/field-cash/utils";
import { Button } from "@/components/ui/button";
import type { AppAbility } from "@/lib/capabilities";

import type { RetakePayment } from "./type";
import {
  RETAKE_BUTTON_TEXT,
  canRetakePhoto,
  retakeButtonLabel,
  retakeSummary,
} from "./utils";

import { RetakePhotoSheet } from "./retake-photo-sheet";

type RetakePhotoButtonProps = {
  orgId: string;
  payment: RetakePayment;
  // On Tagihan hari ini the button sits on its own line beside the payment ("Tunai Rp1.000.000 ·
  // 10.42"); on Setor tunai the receipt row already names it.
  withSummary?: boolean;
};

// "Ambil ulang" beside a photo the scan refused (rejected or infected; never a clean, pending or
// missing one), shown to a member with cash:record (conditional counts; the API decides that only
// the member who recorded the payment may retake its photo). A text button, not a primary. Each
// press mounts the sheet afresh. It is rendered for every payment, so the sheet stays mounted to
// animate out after the list reloads and the button goes.
export function RetakePhotoButton({
  orgId,
  payment,
  withSummary = false,
}: RetakePhotoButtonProps) {
  const ability = useAbility<AppAbility>();
  const [now] = useState(() => new Date());
  const [opening, setOpening] = useState(0);
  const [open, setOpen] = useState(false);
  const summary = retakeSummary(payment.amount, payment.recordedAt, now);
  const shown = canRecordCash(ability) && canRetakePhoto(payment.photoStatus);

  const button = (
    <Button
      type="button"
      variant="ghost"
      aria-label={retakeButtonLabel(summary)}
      onClick={() => {
        setOpening((previous) => previous + 1);
        setOpen(true);
      }}
      className="h-11 shrink-0 rounded-xl px-3 text-[15px] font-semibold text-primary hover:text-primary md:text-base"
    >
      {RETAKE_BUTTON_TEXT}
    </Button>
  );

  return (
    <>
      {shown &&
        (withSummary ? (
          <div className="-my-1 flex items-center justify-between gap-2">
            <span className="text-sm text-muted-foreground tabular-nums md:text-base">
              {summary}
            </span>
            {button}
          </div>
        ) : (
          button
        ))}
      {/* Rendered whatever the photo or the ability says now, so an open sheet is never cut off. */}
      {opening > 0 && (
        <RetakePhotoSheet
          key={opening}
          orgId={orgId}
          payment={payment}
          summary={summary}
          open={open}
          onOpenChange={setOpen}
        />
      )}
    </>
  );
}
