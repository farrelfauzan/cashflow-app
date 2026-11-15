"use client";

import { useAbility } from "@casl/react";
import Link from "next/link";

import {
  canReadCheckQueue,
  checkQueueLinkLabel,
  TITLE,
} from "@/components/client/check-queue/utils";
import { CountBadge } from "@/components/client/deposit-count/count-badge";
import { DEFAULT_PAGE_SIZE } from "@/components/client/list/utils";
import { buttonVariants } from "@/components/ui/button";
import { useListCheckQueue } from "@/lib/api/generated/check-queue/check-queue";
import type { AppAbility } from "@/lib/capabilities";
import { orgPath } from "@/lib/org-path";
import { cn } from "@/lib/utils";

type BillingCheckQueueLinkProps = {
  orgId: string;
};

// Penagihan's link to "Antrean cek", with the number of items waiting as an accent badge (none
// at 0). Only for a member who may answer the items or read cash for the whole organization; the
// API answers anyone else 403. A secondary button, next to "Setoran tunai". It asks for the same
// first page as the screen with no kind chosen, so the screen opens from the cache.
export function BillingCheckQueueLink({ orgId }: BillingCheckQueueLinkProps) {
  const ability = useAbility<AppAbility>();
  const canRead = canReadCheckQueue(ability);
  const queue = useListCheckQueue(
    orgId,
    { page: 1, pageSize: DEFAULT_PAGE_SIZE },
    { query: { enabled: canRead } },
  );
  if (!canRead) return null;
  const count = queue.data?.total ?? 0;

  return (
    <Link
      href={orgPath(orgId, "billing/check-queue")}
      aria-label={checkQueueLinkLabel(count)}
      className={cn(
        buttonVariants({ variant: "outline" }),
        "h-11 w-full rounded-xl bg-card text-base font-semibold lg:w-auto",
      )}
    >
      {TITLE}
      {count > 0 && <CountBadge count={count} />}
    </Link>
  );
}
