"use client";

import { toFormError } from "@/lib/api/form-error";
import { useGetTenantAccount } from "@/lib/api/generated/money/money";
import { SUMMARY_LOADING_TEXT, SUMMARY_TITLE, unpaidSummary } from "./utils";

type PaymentSummaryBoxProps = {
  orgId: string;
  tenantId: string;
};

// The top of "Catat pembayaran": what the tenant owes in total and which invoices are unpaid,
// oldest first, straight from getTenantAccount (the query the page behind already holds).
export function PaymentSummaryBox({ orgId, tenantId }: PaymentSummaryBoxProps) {
  const account = useGetTenantAccount(orgId, tenantId);

  if (account.isError) {
    return (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {toFormError(account.error, []).message}
      </p>
    );
  }
  if (account.data === undefined) {
    return (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {SUMMARY_LOADING_TEXT}
      </p>
    );
  }

  const summary = unpaidSummary(account.data);
  return (
    <section
      aria-label={SUMMARY_TITLE}
      className="flex flex-col gap-1 rounded-[14px] bg-muted px-4 py-3.5"
    >
      <span className="text-[15px] font-semibold text-muted-foreground md:text-base">
        {SUMMARY_TITLE}
      </span>
      <span className="text-[26px] leading-tight font-bold md:text-[28px]">
        {summary.total}
      </span>
      <p className="text-sm leading-normal text-foreground md:text-base">
        {summary.detail}
      </p>
    </section>
  );
}
