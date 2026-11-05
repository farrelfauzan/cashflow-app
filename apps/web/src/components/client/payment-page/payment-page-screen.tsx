"use client";

import { use } from "react";

import { useGetPaymentPage } from "@/lib/api/generated/payment-page/payment-page";

import { PaymentOpenView } from "./payment-open-view";
import { PaymentPageFrame } from "./payment-page-frame";
import { PaymentPageLoading } from "./payment-page-loading";
import { PaymentPageMessage } from "./payment-page-message";
import { PaymentPaidView } from "./payment-paid-view";
import { MESSAGES, isWellFormedToken, paymentPageState } from "./utils";

type PaymentPageScreenProps = {
  params: Promise<{ token: string }>;
};

// The tenant payment page, read in the browser only: the API limits it per client address,
// which a server-side read would hide behind the web server's own address. A malformed token is
// never sent. The page is read again when the tenant comes back to the tab (after paying in
// their bank's app), never on a timer.
export function PaymentPageScreen({ params }: PaymentPageScreenProps) {
  const { token } = use(params);
  const wellFormed = isWellFormedToken(token);
  const page = useGetPaymentPage(token, {
    query: {
      enabled: wellFormed,
      // Any focus after the first read re-reads; 404, 410 and 429 are never retried
      // (shouldRetryQuery in the query client).
      staleTime: 0,
      refetchOnWindowFocus: true,
      refetchInterval: false,
    },
  });

  const state = paymentPageState({
    wellFormed,
    data: page.data,
    error: page.error,
  });

  if (state.kind === "loading") return <PaymentPageLoading />;
  if (state.kind === "error") {
    return (
      <PaymentPageFrame>
        <PaymentPageMessage
          message={MESSAGES[state.error]}
          onRetry={() => void page.refetch()}
        />
      </PaymentPageFrame>
    );
  }
  return (
    <PaymentPageFrame>
      {state.view.status === "paid" ? (
        <PaymentPaidView view={state.view} />
      ) : (
        <PaymentOpenView view={state.view} />
      )}
    </PaymentPageFrame>
  );
}
