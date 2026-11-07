"use client";

import { ExternalLink, LoaderCircle } from "lucide-react";

import { FormAlert } from "@/components/client/form/form-alert";
import {
  TELEGRAM_LINK_TEXT,
  TELEGRAM_OPEN_HINT,
  TELEGRAM_OPEN_TEXT,
} from "@/components/client/payment-page/utils";
import { Button } from "@/components/ui/button";

import { useMyTelegramLink } from "./use-my-telegram-link";
import {
  STAFF_TELEGRAM_UNLINKING_TEXT,
  STAFF_TELEGRAM_UNLINK_TEXT,
} from "./utils";

type TelegramDialogBodyProps = {
  linked: boolean;
};

// Not linked: "Hubungkan Telegram" asks for a one-time deep link, then "Buka Telegram" opens it
// in a new tab or the Telegram app, without a referrer, until its code expires. Linked:
// "Putuskan Telegram". The dialog's one action is primary; unlinking stays outline.
export function TelegramDialogBody({ linked }: TelegramDialogBodyProps) {
  const link = useMyTelegramLink();

  return (
    <div className="flex flex-col gap-3">
      {link.alert !== null && <FormAlert message={link.alert} />}
      {linked ? (
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="h-12 w-full font-semibold"
          disabled={link.unlinking}
          onClick={link.unlink}
        >
          {link.unlinking && (
            <LoaderCircle aria-hidden="true" className="animate-spin" />
          )}
          {link.unlinking
            ? STAFF_TELEGRAM_UNLINKING_TEXT
            : STAFF_TELEGRAM_UNLINK_TEXT}
        </Button>
      ) : link.state.kind === "ready" ? (
        <>
          <Button asChild size="lg" className="h-12 w-full font-semibold">
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
          size="lg"
          className="h-12 w-full font-semibold"
          disabled={link.state.kind === "asking"}
          onClick={link.ask}
        >
          {link.state.kind === "asking" && (
            <LoaderCircle aria-hidden="true" className="animate-spin" />
          )}
          {TELEGRAM_LINK_TEXT}
        </Button>
      )}
    </div>
  );
}
