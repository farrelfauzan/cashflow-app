"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import {
  useGetMyFieldCash,
  useListMyCashDeposits,
  useListMyUndepositedCash,
} from "@/lib/api/generated/field/field";
import type { AppAbility } from "@/lib/capabilities";

import {
  canDepositCash,
  cashSinceLine,
  DEPOSITS_PAGE_SIZE,
  FORBIDDEN_TEXT,
  HELD_PAGE_SIZE,
  isForbidden,
  LOADING_TEXT,
  pastDepositViews,
  pendingDeposits,
  pendingDepositView,
} from "./utils";

import { CashDepositForm } from "./cash-deposit-form";
import { CashHeldCard } from "./cash-held-card";
import { DepositEmpty } from "./deposit-empty";
import { HeldReceiptList } from "./held-receipt-list";
import { PastDepositList } from "./past-deposit-list";
import { PendingDepositCard } from "./pending-deposit-card";

type CashDepositViewProps = {
  orgId: string;
};

// The screen's content, from three answers of the API: the cash held (getMyFieldCash), the
// receipts it adds up (listMyUndepositedCash) and the karyawan's deposits (listMyCashDeposits).
// With cash held: the total, the receipts and the form (board phone 1). Each deposit waiting for
// an admin: its card (board phone 2). Then the counted deposits. With nothing held and nothing
// waiting: a calm empty line, no button. Shown to a member whose capabilities grant cash:deposit;
// the API checks the cash is theirs.
export function CashDepositView({ orgId }: CashDepositViewProps) {
  const ability = useAbility<AppAbility>();
  const canDeposit = canDepositCash(ability);
  const query = { query: { enabled: canDeposit } };
  const cash = useGetMyFieldCash(orgId, query);
  const held = useListMyUndepositedCash(
    orgId,
    { pageSize: HELD_PAGE_SIZE },
    query,
  );
  const deposits = useListMyCashDeposits(
    orgId,
    { pageSize: DEPOSITS_PAGE_SIZE },
    query,
  );
  // A new form (and so a new requestId) after every deposit recorded.
  const [round, setRound] = useState(0);

  const failed = [cash, held, deposits].find((answer) => answer.error !== null);
  if (!canDeposit || [cash, held, deposits].some((a) => isForbidden(a.error))) {
    return (
      <p role="alert" className="text-base">
        {FORBIDDEN_TEXT}
      </p>
    );
  }
  if (failed !== undefined) {
    return (
      <TenantLoadError
        message={toFormError(failed.error, []).message}
        onRetry={() => {
          void cash.refetch();
          void held.refetch();
          void deposits.refetch();
        }}
      />
    );
  }
  if (
    cash.data === undefined ||
    held.data === undefined ||
    deposits.data === undefined
  ) {
    return (
      <p className="text-base text-muted-foreground" aria-live="polite">
        {LOADING_TEXT}
      </p>
    );
  }

  const holding = cash.data.count > 0;
  const pending = pendingDeposits(deposits.data.items);
  const past = pastDepositViews(deposits.data.items);

  return (
    <div className="flex flex-col gap-3 md:gap-4">
      {holding && (
        <>
          <CashHeldCard
            sinceLine={cashSinceLine(deposits.data.items[0])}
            amount={cash.data.amount}
            count={cash.data.count}
          />
          <HeldReceiptList items={held.data.items} total={held.data.total} />
          <CashDepositForm
            key={round}
            orgId={orgId}
            cash={cash.data}
            onSaved={() => setRound((current) => current + 1)}
          />
        </>
      )}
      {pending.map((deposit) => (
        <PendingDepositCard
          key={deposit.id}
          view={pendingDepositView(deposit)}
        />
      ))}
      {!holding && pending.length === 0 && <DepositEmpty />}
      {past.length > 0 && <PastDepositList views={past} />}
    </div>
  );
}
