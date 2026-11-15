"use client";

import { useAbility } from "@casl/react";
import Link from "next/link";

import { CountBadge } from "@/components/client/deposit-count/count-badge";
import {
  canReadDeposits,
  DEPOSIT_LINK_TEXT,
  depositLinkLabel,
} from "@/components/client/deposit-count/utils";
import { DEFAULT_PAGE_SIZE } from "@/components/client/list/utils";
import { buttonVariants } from "@/components/ui/button";
import { useListCashDeposits } from "@/lib/api/generated/money/money";
import type { AppAbility } from "@/lib/capabilities";
import { orgPath } from "@/lib/org-path";
import { cn } from "@/lib/utils";

type BillingDepositLinkProps = {
  orgId: string;
};

// Penagihan's link to "Setoran tunai", with the number of deposits waiting to be counted as an
// accent badge (none at 0). Only for a member who may read deposits for the whole organization
// (count them, or read cash); the API answers anyone else 403. A secondary button, so Penagihan
// keeps no primary action. It asks for the same first page as the screen's pending tab, so the
// screen opens from the cache.
export function BillingDepositLink({ orgId }: BillingDepositLinkProps) {
  const ability = useAbility<AppAbility>();
  const canRead = canReadDeposits(ability);
  const pending = useListCashDeposits(
    orgId,
    { status: "pending", page: 1, pageSize: DEFAULT_PAGE_SIZE },
    { query: { enabled: canRead } },
  );
  if (!canRead) return null;
  const count = pending.data?.total ?? 0;

  return (
    <Link
      href={orgPath(orgId, "billing/deposits")}
      aria-label={depositLinkLabel(count)}
      className={cn(
        buttonVariants({ variant: "outline" }),
        "h-11 w-full rounded-xl bg-card text-base font-semibold lg:w-auto",
      )}
    >
      {DEPOSIT_LINK_TEXT}
      {count > 0 && <CountBadge count={count} />}
    </Link>
  );
}
