"use client";

import { useRef } from "react";

import type {
  MakePaymentPromiseRequest,
  PaymentPage,
} from "@/lib/api/generated/model";
import { useMakePaymentPromise } from "@/lib/api/generated/payment-page/payment-page";

import type { MakePromise, PromiseSaveResult } from "./type";
import { promiseErrorView } from "./utils";

type UseMakePromiseInput = {
  // The link's token: it goes only into the generated request's path.
  token: string;
  // Reads getPaymentPage again, so the page shows the promise (or what changed).
  refetchPage: () => Promise<PaymentPage | undefined>;
  // The request found that the link no longer works.
  onLinkGone: () => void;
};

// "Simpan janji" (makePaymentPromise). Never retried on its own (a rate limit shares the page's
// budget), and one save at a time: a double tap sends one request. After a save, or an answer
// that says the page is out of date, the page is read again.
export function useMakePromise({
  token,
  refetchPage,
  onLinkGone,
}: UseMakePromiseInput): MakePromise {
  const mutation = useMakePaymentPromise();
  const running = useRef(false);

  async function save(
    body: MakePaymentPromiseRequest,
  ): Promise<PromiseSaveResult> {
    if (running.current) return { kind: "busy" };
    running.current = true;
    try {
      const promise = await mutation.mutateAsync({ token, data: body });
      void refetchPage();
      return { kind: "saved", promise };
    } catch (error) {
      const view = promiseErrorView(error, body.kind);
      if (view.kind === "link-gone") onLinkGone();
      else if (view.refetchPage) void refetchPage();
      return { kind: "failed", error: view, cause: error };
    } finally {
      running.current = false;
    }
  }

  return { save, pending: mutation.isPending };
}
