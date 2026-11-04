"use client";

import { Check, Lock, TriangleAlert } from "lucide-react";

import { cn } from "@/lib/utils";

type MeterStatusBoxProps = {
  // "warning": an amount is already recorded; "success": it was just saved; "locked": an invoice
  // bills it.
  tone: "warning" | "success" | "locked";
  text: string;
};

// A status line of "Catat meteran", announced to screen readers.
export function MeterStatusBox({ tone, text }: MeterStatusBoxProps) {
  const Icon =
    tone === "success" ? Check : tone === "locked" ? Lock : TriangleAlert;
  return (
    <div
      role="status"
      className={cn(
        "flex items-start gap-2.5 rounded-xl px-3.5 py-3 text-foreground",
        tone === "warning" && "border border-warning bg-warning/15",
        tone === "success" && "bg-accent",
        tone === "locked" && "bg-muted",
      )}
    >
      <Icon
        aria-hidden="true"
        className={cn(
          "mt-0.5 size-5 shrink-0",
          tone === "warning" && "text-warning",
          tone === "success" && "text-primary",
        )}
      />
      <p className="text-sm leading-normal font-semibold md:text-base">
        {text}
      </p>
    </div>
  );
}
