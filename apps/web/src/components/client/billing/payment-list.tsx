"use client";

import type { PaymentRowView } from "./type";
import {
  ACCOUNT_CARD_CLASS,
  NO_PAYMENTS_TEXT,
  PAYMENTS_LIST_LABEL,
  PAYMENTS_TITLE,
} from "./utils";

import { PaymentRow } from "./payment-row";

type PaymentListProps = {
  payments: readonly PaymentRowView[];
  // Opens "Koreksi nominal" for one payment, by id; none hides the action.
  onCorrect?: (paymentId: string) => void;
};

// "Pembayaran dan alokasi": every payment of the tenant, newest first, with what it paid.
export function PaymentList({ payments, onCorrect }: PaymentListProps) {
  return (
    <section className={ACCOUNT_CARD_CLASS}>
      <h2 className="text-xl font-bold md:text-2xl">{PAYMENTS_TITLE}</h2>
      {payments.length === 0 ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {NO_PAYMENTS_TEXT}
        </p>
      ) : (
        <ul aria-label={PAYMENTS_LIST_LABEL} className="flex flex-col gap-3">
          {payments.map((payment) => (
            <PaymentRow
              key={payment.id}
              payment={payment}
              onCorrect={
                onCorrect === undefined
                  ? undefined
                  : () => onCorrect(payment.id)
              }
            />
          ))}
        </ul>
      )}
    </section>
  );
}
