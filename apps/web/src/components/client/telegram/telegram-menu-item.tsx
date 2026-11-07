"use client";

import { Send } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { useGetMyTelegram } from "@/lib/api/generated/auth/auth";

import { TelegramDialog } from "./telegram-dialog";
import { STAFF_TELEGRAM_MENU_TEXT } from "./utils";

// "Telegram" at the foot of the side menu, above "Keluar" (decision 42, no board, approved by the
// orchestrator): only where the environment has a bot, so production never shows it. Read again
// when the member comes back to the tab, which is how a Start pressed in Telegram shows up.
export function TelegramMenuItem() {
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
        className="justify-start px-2 font-semibold text-muted-foreground rail:justify-center rail:px-0"
        onClick={() => setOpen(true)}
      >
        <Send aria-hidden="true" className="size-5" />
        <span className="rail:sr-only">{STAFF_TELEGRAM_MENU_TEXT}</span>
      </Button>
      <TelegramDialog
        open={open}
        onOpenChange={setOpen}
        linked={telegram.data.linked}
      />
    </>
  );
}
