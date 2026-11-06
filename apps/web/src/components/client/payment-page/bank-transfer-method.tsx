"use client";

import { BankTransferSection } from "./bank-transfer-section";
import { NoTransferNote } from "./no-transfer-note";
import type { TransferView } from "./type";

type BankTransferMethodProps = {
  transfer: TransferView | null;
  noTransferText: string;
};

// Paying by bank transfer: the organization's account, or whom to contact when none is set.
export function BankTransferMethod({
  transfer,
  noTransferText,
}: BankTransferMethodProps) {
  if (transfer === null) return <NoTransferNote text={noTransferText} />;
  return <BankTransferSection transfer={transfer} />;
}
