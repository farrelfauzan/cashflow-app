"use client";

import { CircleCheck, ExternalLink, LoaderCircle } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { TelegramOfferView } from "./type";
import { useTelegramLink } from "./use-telegram-link";
import {
  TELEGRAM_LINKED_TEXT,
  TELEGRAM_LINK_TEXT,
  TELEGRAM_OPEN_HINT,
  TELEGRAM_OPEN_TEXT,
  TELEGRAM_TEXT,
  TELEGRAM_TITLE,
} from "./utils";

type TelegramCardProps = {
  offer: TelegramOfferView;
  // The link's token: it goes only into the generated request's path.
  token: string;
  onLinkGone: () => void;
};

// "Pengingat lewat Telegram" on Bayar-Pilih (no board; a demo channel, decision 42). A quiet card,
// never a second primary: "Hubungkan Telegram" asks for a one-time deep link, then "Buka Telegram"
// opens it in a new tab or the Telegram app, without a referrer. Once the tenant pressed Start,
// the page (read again when they come back) says the link is made.
export function TelegramCard({ offer, token, onLinkGone }: TelegramCardProps) {
  const link = useTelegramLink({ token, onLinkGone });
  if (link.hidden) return null;

  return (
    <section
      aria-label={TELEGRAM_TITLE}
      className="flex flex-col gap-2.5 rounded-xl border bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4"
    >
      <h2 className="text-sm font-semibold md:text-base">{TELEGRAM_TITLE}</h2>
      {offer.linked ? (
        <p className="flex items-start gap-2 text-sm md:text-base">
          <CircleCheck
            aria-hidden="true"
            className="mt-0.5 size-5 shrink-0 text-primary"
          />
          {TELEGRAM_LINKED_TEXT}
        </p>
      ) : (
        <>
          <p className="text-sm text-muted-foreground md:text-base">
            {TELEGRAM_TEXT}
          </p>
          {link.state.kind === "ready" ? (
            <>
              <Button
                asChild
                variant="outline"
                size="lg"
                className="w-full font-semibold"
              >
                <a
                  href={link.state.deepLink}
                  target="_blank"
                  rel="noopener noreferrer"
                  referrerPolicy="no-referrer"
                >
                  <ExternalLink aria-hidden="true" />
                  {TELEGRAM_OPEN_TEXT}
                </a>
              </Button>
              <p className="text-sm text-muted-foreground md:text-base">
                {TELEGRAM_OPEN_HINT}
              </p>
            </>
          ) : (
            <Button
              type="button"
              variant="outline"
              size="lg"
              className="w-full font-semibold"
              disabled={link.state.kind === "asking"}
              onClick={link.ask}
            >
              {link.state.kind === "asking" && (
                <LoaderCircle aria-hidden="true" className="animate-spin" />
              )}
              {TELEGRAM_LINK_TEXT}
            </Button>
          )}
          {link.alert !== null && (
            <p role="alert" className="text-sm text-destructive md:text-base">
              {link.alert}
            </p>
          )}
        </>
      )}
    </section>
  );
}
