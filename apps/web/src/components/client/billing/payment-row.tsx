"use client";

import type { PaymentRowView } from "./type";
import { ALL_CREDIT_TEXT } from "./utils";

import { CorrectButton } from "./correct-button";

type PaymentRowProps = {
  payment: PaymentRowView;
  // Opens "Koreksi nominal" for this payment; none hides the action.
  onCorrect?: () => void;
};

// One payment: when and how it came in, what it counts for, who recorded it, and the invoices
// it paid ("→ Oktober Rp750.000 (lunas)"), and "Koreksi" next to the amount for a member who may
// correct.
export function PaymentRow({ payment, onCorrect }: PaymentRowProps) {
  return (
    <li className="flex flex-col gap-1.5 border-t pt-3 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-1">
        <h3 className="text-base font-bold md:text-[17px]">{payment.title}</h3>
        <div className="flex items-center gap-3">
          <span className="text-base font-bold md:text-[17px]">
            {payment.amount}
          </span>
          {onCorrect !== undefined && (
            <CorrectButton
              label={payment.correctionTitle}
              onClick={onCorrect}
            />
          )}
        </div>
      </div>
      {payment.correctedFrom !== null && (
        <span className="text-sm text-muted-foreground md:text-base">
          {payment.correctedFrom}
        </span>
      )}
      {payment.note !== null && (
        <p className="text-[15px] md:text-base">{payment.note}</p>
      )}
      {payment.recordedBy !== null && (
        <span className="text-sm text-muted-foreground md:text-base">
          {payment.recordedBy}
        </span>
      )}
      {payment.allocations.length === 0 ? (
        <p className="text-[15px] font-semibold md:text-base">
          {ALL_CREDIT_TEXT}
        </p>
      ) : (
        <ul
          aria-label={`Alokasi ${payment.title}`}
          className="flex flex-col gap-0.5"
        >
          {payment.allocations.map((allocation) => (
            <li
              key={allocation.invoiceId}
              className="text-[15px] font-semibold md:text-base"
            >
              {allocation.text}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
