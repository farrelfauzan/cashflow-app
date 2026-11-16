"use client";

import Link from "next/link";

import { CashHeldCard } from "@/components/client/deposit/cash-held-card";
import { Button } from "@/components/ui/button";
import type { CashTotal } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/org-path";

import { CASH_CARD_LABEL, DEPOSIT_LINK_TEXT } from "./utils";

type FieldCashHeldProps = {
  orgId: string;
  cash: CashTotal;
};

// "Tunai di tangan, belum disetor" at the top of Tagihan hari ini (board HP-Tagihan): Setor
// tunai's card in its strong look, with "Setor ke admin" to Setor tunai while any cash is held.
export function FieldCashHeld({ orgId, cash }: FieldCashHeldProps) {
  return (
    <CashHeldCard
      tone="strong"
      sinceLine={CASH_CARD_LABEL}
      amount={cash.amount}
      count={cash.count}
      action={
        cash.count > 0 ? (
          <Button
            asChild
            variant="outline"
            className="h-11 w-full rounded-[10px] border-0 bg-card text-base font-bold text-primary-strong hover:bg-accent"
          >
            <Link href={orgPath(orgId, "field/deposit")}>
              {DEPOSIT_LINK_TEXT}
            </Link>
          </Button>
        ) : undefined
      }
    />
  );
}
