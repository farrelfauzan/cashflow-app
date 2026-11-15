"use client";

import { useId } from "react";

import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useListCashDepositPayments } from "@/lib/api/generated/money/money";

import {
  depositReceiptView,
  LOADING_TEXT,
  moreReceiptsText,
  RECEIPTS_PAGE_SIZE,
  RECEIPTS_TITLE,
} from "./utils";

import { DepositReceiptRow } from "./deposit-receipt-row";

type DepositReceiptListProps = {
  orgId: string;
  depositId: string;
};

// "Kwitansi dalam setoran ini": the payments the deposit covers, oldest first (the API's order),
// one page of at most 100 with "dan N kwitansi lainnya" under it.
export function DepositReceiptList({
  orgId,
  depositId,
}: DepositReceiptListProps) {
  const titleId = useId();
  const payments = useListCashDepositPayments(orgId, depositId, {
    pageSize: RECEIPTS_PAGE_SIZE,
  });
  const more =
    payments.data === undefined
      ? null
      : moreReceiptsText(payments.data.total, payments.data.items.length);

  return (
    <section aria-labelledby={titleId} className="flex flex-col gap-2">
      <h3
        id={titleId}
        className="text-[15px] leading-normal font-semibold md:text-base"
      >
        {RECEIPTS_TITLE}
      </h3>
      {payments.error !== null ? (
        <TenantLoadError
          message={toFormError(payments.error, []).message}
          onRetry={() => void payments.refetch()}
        />
      ) : payments.data === undefined ? (
        <p className="text-base text-muted-foreground">{LOADING_TEXT}</p>
      ) : (
        <>
          <ul
            aria-labelledby={titleId}
            className="flex flex-col rounded-xl border-[1.5px] bg-card"
          >
            {payments.data.items.map((payment) => (
              <DepositReceiptRow
                key={payment.paymentId}
                view={depositReceiptView(payment)}
              />
            ))}
          </ul>
          {more !== null && (
            <p className="px-1 text-sm text-muted-foreground md:text-base">
              {more}
            </p>
          )}
        </>
      )}
    </section>
  );
}
