"use client";

import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ChooseAction } from "./type";

type ImportChooseActionProps = {
  action: ChooseAction;
  // The check is on its way: the button waits, and never sends twice.
  busy: boolean;
  onCheck: () => void;
  onPick: () => void;
  className?: string;
};

// Step 1's one primary action: "Periksa berkas" (disabled until a file is chosen), or "Pilih
// berkas lain" when the chosen file cannot go on.
export function ImportChooseAction({
  action,
  busy,
  onCheck,
  onPick,
  className,
}: ImportChooseActionProps) {
  const classes = cn(
    "h-12 rounded-xl px-5 text-[17px] font-semibold",
    className,
  );
  if (action === "chooseOther") {
    return (
      <Button type="button" onClick={onPick} className={classes}>
        <RefreshCw aria-hidden="true" className="size-5" strokeWidth={2} />
        Pilih berkas lain
      </Button>
    );
  }
  return (
    <Button
      type="button"
      onClick={onCheck}
      disabled={action === "checkDisabled" || busy}
      aria-busy={busy}
      className={classes}
    >
      {busy ? "Memeriksa…" : "Periksa berkas"}
    </Button>
  );
}
