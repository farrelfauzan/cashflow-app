"use client";

import { useEffect, useRef, useState } from "react";

import type { PaymentPage } from "@/lib/api/generated/model";
import {
  getPaymentStatus,
  useCheckPaymentQris,
  useCreatePaymentQris,
  useGetPaymentStatus,
} from "@/lib/api/generated/payment-page/payment-page";

import type {
  QrisEvent,
  QrisModel,
  QrisOfferView,
  QrisPayment,
  QrisStep,
} from "./type";
import {
  INITIAL_QRIS_MODEL,
  QRIS_OVERLOADED_RETRY_MS,
  canCheckQris,
  canStartQris,
  qrisCreateAmount,
  qrisStep,
  readsStatus,
  statusPollInterval,
} from "./utils";

type UseQrisPaymentInput = {
  // The link's token: it goes only into the generated requests' paths.
  token: string;
  offer: QrisOfferView;
  // Reads getPaymentPage again; resolves to the latest page, or undefined when there is none.
  refetchPage: () => Promise<PaymentPage | undefined>;
  // A QRIS request found that the link no longer works.
  onLinkGone: () => void;
};

// The QRIS part of the payment page: the state machine of utils.ts (qrisStep), the two
// mutations and the status read. getPaymentStatus is read once on load, then polled while a QR
// of this visit is pending (5 s) or being checked (15 s), never while the page is hidden. Every
// answer goes through qrisStep, and nothing is retried on its own.
export function useQrisPayment({
  token,
  offer,
  refetchPage,
  onLinkGone,
}: UseQrisPaymentInput): QrisPayment {
  const [model, setModel] = useState<QrisModel>(INITIAL_QRIS_MODEL);
  // The model as of the last event, for handlers and answers that land between renders: a
  // double tap sees the first tap's "creating" at once.
  const current = useRef<QrisModel>(INITIAL_QRIS_MODEL);

  const create = useCreatePaymentQris();
  const confirm = useCheckPaymentQris();

  // The one automatic retry after qris_overloaded, cleared when the page goes away.
  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (retryTimer.current !== null) clearTimeout(retryTimer.current);
    },
    [],
  );

  // Moves the state machine and runs its effects; returns the step, whose pollNow and
  // retryCreate only the caller can act on.
  function apply(event: QrisEvent): QrisStep {
    const next = qrisStep(current.current, event);
    current.current = next.model;
    setModel(next.model);
    if (next.refetchPage) void refetchPage();
    if (next.linkGone) onLinkGone();
    return next;
  }

  const status = useGetPaymentStatus(token, {
    query: {
      enabled: readsStatus(model.card),
      refetchInterval: statusPollInterval(model.card),
      // Polling pauses while the page is hidden and goes on when the tenant comes back.
      refetchIntervalInBackground: false,
      refetchOnWindowFocus: true,
      staleTime: 0,
      // A failed read waits for the next tick; it is never repeated at once (the rate budget).
      retry: false,
      // Each answer is an event for the state machine; the cached value itself is not shown.
      queryFn: async ({ signal }) => {
        try {
          const answer = await getPaymentStatus(token, undefined, signal);
          apply({ type: "status-read", status: answer, now: Date.now() });
          return answer;
        } catch (error) {
          if (!signal.aborted) apply({ type: "status-failed", error });
          throw error;
        }
      },
    },
  });

  function pollNow() {
    if (!readsStatus(current.current.card)) return;
    void status.refetch();
  }

  async function start(afterExpiry: boolean) {
    if (!canStartQris(current.current)) return;
    apply({ type: "create-started" });

    let outstanding = offer.outstanding;
    if (afterExpiry) {
      const page = await refetchPage();
      if (page !== undefined) outstanding = page.paid ? 0 : page.outstanding;
    }
    const amount = qrisCreateAmount(outstanding);
    if (amount === null) {
      apply({ type: "create-skipped" });
      return;
    }

    // One automatic retry after qris_overloaded (nothing reached AutoGoPay), never more.
    for (;;) {
      try {
        const answer = await create.mutateAsync({ token, data: { amount } });
        apply({ type: "created", status: answer, now: Date.now() });
        return;
      } catch (error) {
        const next = apply({ type: "create-failed", error, now: Date.now() });
        if (!next.retryCreate) return;
      }
      await new Promise<void>((resolve) => {
        retryTimer.current = setTimeout(resolve, QRIS_OVERLOADED_RETRY_MS);
      });
      retryTimer.current = null;
      if (current.current.card.kind !== "creating") return;
    }
  }

  async function check() {
    if (!canCheckQris(current.current)) return;
    apply({ type: "check-started" });
    try {
      const answer = await confirm.mutateAsync({ token });
      apply({ type: "checked", status: answer, now: Date.now() });
    } catch (error) {
      if (apply({ type: "check-failed", error }).pollNow) pollNow();
    }
  }

  return {
    model,
    start: (afterExpiry) => void start(afterExpiry),
    check: () => void check(),
    pollNow,
    selectTab: (tab) => {
      apply({ type: "tab-selected", tab });
    },
  };
}
