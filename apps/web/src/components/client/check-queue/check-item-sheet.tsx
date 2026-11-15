"use client";

import type { CheckQueueItem } from "@/lib/api/generated/model";

import { DepositGapSheet } from "./deposit-gap-sheet";
import { PhoneChangeSheet } from "./phone-change-sheet";
import { QrisMismatchSheet } from "./qris-mismatch-sheet";
import { UploadInfectedSheet } from "./upload-infected-sheet";

type CheckItemSheetProps = {
  orgId: string;
  item: CheckQueueItem;
  // Whether the member may answer items; otherwise every drawer is read-only.
  canAct: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// The drawer of the item's kind. A transfer proof has none yet (its rows do not open one), nor
// does an item the API sent without its kind's object.
export function CheckItemSheet({
  orgId,
  item,
  canAct,
  open,
  onOpenChange,
}: CheckItemSheetProps) {
  const shared = { orgId, itemId: item.id, canAct, open, onOpenChange };
  if (item.depositGap !== undefined) {
    return <DepositGapSheet {...shared} gap={item.depositGap} />;
  }
  if (item.uploadInfected !== undefined) {
    return <UploadInfectedSheet {...shared} upload={item.uploadInfected} />;
  }
  if (item.qrisAmountMismatch !== undefined) {
    return <QrisMismatchSheet {...shared} mismatch={item.qrisAmountMismatch} />;
  }
  if (item.phoneChanged !== undefined) {
    return (
      <PhoneChangeSheet
        {...shared}
        createdAt={item.createdAt}
        change={item.phoneChanged}
      />
    );
  }
  return null;
}
