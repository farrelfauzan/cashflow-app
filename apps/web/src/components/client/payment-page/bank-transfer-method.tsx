"use client";

import { BankTransferSection } from "./bank-transfer-section";
import { NoTransferNote } from "./no-transfer-note";
import type { ProofSender, TransferView } from "./type";

type BankTransferMethodProps = {
  transfer: TransferView | null;
  noTransferText: string;
  proof: ProofSender;
};

// Paying by bank transfer: the organization's account, or whom to contact when none is set.
export function BankTransferMethod({
  transfer,
  noTransferText,
  proof,
}: BankTransferMethodProps) {
  if (transfer === null) return <NoTransferNote text={noTransferText} />;
  return <BankTransferSection transfer={transfer} proof={proof} />;
}
