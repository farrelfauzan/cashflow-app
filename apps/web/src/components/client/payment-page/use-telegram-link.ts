"use client";

import { useState } from "react";

import { useLinkPaymentPageTelegram } from "@/lib/api/generated/payment-page/payment-page";

import type { TelegramLinkState } from "./type";
import { GENERIC_ERROR_MESSAGE } from "@/lib/api/errors";
import { isTelegramDeepLink, telegramErrorView } from "./utils";

type UseTelegramLinkInput = {
  // The link's token: it goes only into the generated request's path.
  token: string;
  // The request found that the link no longer works.
  onLinkGone: () => void;
};

type TelegramLink = {
  state: TelegramLinkState;
  // The API's message (or the page's own) in the card; null when there is none.
  alert: string | null;
  // The environment has no bot after all: the card goes.
  hidden: boolean;
  ask: () => void;
};

// "Hubungkan Telegram" (linkPaymentPageTelegram): one request at a time, never retried on its own
// (a rate limit shares the page's budget). The answer's deep link is kept only in this state, to
// be opened by the tenant; it is never logged.
export function useTelegramLink({
  token,
  onLinkGone,
}: UseTelegramLinkInput): TelegramLink {
  const mutation = useLinkPaymentPageTelegram();
  const [state, setState] = useState<TelegramLinkState>({ kind: "idle" });
  const [alert, setAlert] = useState<string | null>(null);
  const [hidden, setHidden] = useState(false);

  function ask() {
    if (state.kind === "asking") return;
    setState({ kind: "asking" });
    setAlert(null);
    mutation.mutate(
      { token },
      {
        onSuccess: (answer) => {
          if (isTelegramDeepLink(answer.deepLink)) {
            setState({ kind: "ready", deepLink: answer.deepLink });
            return;
          }
          setState({ kind: "idle" });
          setAlert(GENERIC_ERROR_MESSAGE);
        },
        onError: (error) => {
          setState({ kind: "idle" });
          const view = telegramErrorView(error);
          if (view.kind === "link-gone") onLinkGone();
          else if (view.kind === "hidden") setHidden(true);
          else setAlert(view.message);
        },
      },
    );
  }

  return { state, alert, hidden, ask };
}
