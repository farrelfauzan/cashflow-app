"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import type { TelegramLinkState } from "@/components/client/payment-page/type";
import {
  isTelegramDeepLink,
  telegramLinkDelay,
  telegramLinkExpiresAt,
} from "@/components/client/payment-page/utils";
import { GENERIC_ERROR_MESSAGE } from "@/lib/api/errors";
import { toFormError } from "@/lib/api/form-error";
import {
  getGetMyTelegramQueryKey,
  useLinkMyTelegram,
  useUnlinkMyTelegram,
} from "@/lib/api/generated/auth/auth";

import { staffTelegramErrorView } from "./utils";

type MyTelegramLink = {
  state: TelegramLinkState;
  // The API's message in the dialog; null when there is none.
  alert: string | null;
  unlinking: boolean;
  ask: () => void;
  unlink: () => void;
};

// "Hubungkan Telegram" (linkMyTelegram) and "Putuskan Telegram" (unlinkMyTelegram), one request
// at a time. As on the payment page, the deep link is kept only in this state until its code
// expires, for the member to open; it is never logged. Both refresh getMyTelegram (never patch
// it): unlinking says so at once, and a missing bot hides the menu item.
export function useMyTelegramLink(): MyTelegramLink {
  const queryClient = useQueryClient();
  function refresh() {
    return queryClient.invalidateQueries({
      queryKey: getGetMyTelegramQueryKey(),
    });
  }
  // The refreshes sit on the hooks, not on mutate(): TanStack runs these even after the dialog
  // closes mid-request, so the menu item never keeps a stale "linked".
  const link = useLinkMyTelegram({
    mutation: {
      onError: (error) => {
        if (staffTelegramErrorView(error).kind === "hidden") void refresh();
      },
    },
  });
  const unlinkRequest = useUnlinkMyTelegram({
    mutation: { onSettled: () => void refresh() },
  });
  const [state, setState] = useState<TelegramLinkState>({ kind: "idle" });
  const [alert, setAlert] = useState<string | null>(null);

  const expiresAt = state.kind === "ready" ? state.expiresAt : null;
  useEffect(() => {
    if (expiresAt === null) return;
    const timer = window.setTimeout(
      () => setState({ kind: "idle" }),
      telegramLinkDelay(expiresAt, Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [expiresAt]);

  function ask() {
    if (state.kind === "asking" || unlinkRequest.isPending) return;
    const askedAt = Date.now();
    setState({ kind: "asking" });
    setAlert(null);
    link.mutate(undefined, {
      onSuccess: (answer) => {
        if (isTelegramDeepLink(answer.deepLink)) {
          setState({
            kind: "ready",
            deepLink: answer.deepLink,
            expiresAt: telegramLinkExpiresAt(askedAt),
          });
          return;
        }
        setState({ kind: "idle" });
        setAlert(GENERIC_ERROR_MESSAGE);
      },
      onError: (error) => {
        setState({ kind: "idle" });
        const view = staffTelegramErrorView(error);
        if (view.kind === "alert") setAlert(view.message);
      },
    });
  }

  function unlink() {
    if (state.kind === "asking" || unlinkRequest.isPending) return;
    setAlert(null);
    unlinkRequest.mutate(undefined, {
      onSuccess: () => setState({ kind: "idle" }),
      onError: (error) => setAlert(toFormError(error, []).message),
    });
  }

  return {
    state,
    alert,
    unlinking: unlinkRequest.isPending,
    ask,
    unlink,
  };
}
