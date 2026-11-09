"use client";

import { ChevronRight, Send } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { useGetMyTelegram } from "@/lib/api/generated/auth/auth";

import { TelegramDialog } from "./telegram-dialog";
import type { TelegramMenuItemVariant } from "./type";
import { STAFF_TELEGRAM_MENU_TEXT, TELEGRAM_MENU_ITEM_CLASS } from "./utils";

// "Telegram" at the foot of the side menu, above "Keluar" (decision 42, no board, approved by the
// orchestrator): only where the environment has a bot, so production never shows it. Read again
// when the member comes back to the tab, which is how a Start pressed in Telegram shows up.
type TelegramMenuItemProps = {
  variant?: TelegramMenuItemVariant;
};

export function TelegramMenuItem({ variant = "menu" }: TelegramMenuItemProps) {
  const [open, setOpen] = useState(false);
  const telegram = useGetMyTelegram({
    query: { staleTime: 0, refetchOnWindowFocus: true },
  });
  if (telegram.data?.available !== true) return null;

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        className={TELEGRAM_MENU_ITEM_CLASS[variant]}
        onClick={() => setOpen(true)}
      >
        <Send aria-hidden="true" className="size-5" />
        <span className="flex-1 text-left rail:sr-only">
          {STAFF_TELEGRAM_MENU_TEXT}
        </span>
        {variant === "sheet" && (
          <ChevronRight aria-hidden="true" className="size-5" />
        )}
      </Button>
      <TelegramDialog
        open={open}
        onOpenChange={setOpen}
        linked={telegram.data.linked}
      />
    </>
  );
}
