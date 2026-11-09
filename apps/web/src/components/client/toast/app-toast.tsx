"use client";

import { XIcon } from "lucide-react";
import { toast } from "sonner";

import { cn } from "@/lib/utils";

import type { ToastMessage } from "./type";
import { TOAST_CLOSE_LABEL, TOAST_TONES } from "./utils";

type AppToastProps = ToastMessage & {
  // Sonner's id, for the close button.
  id: string | number;
};

// One toast as the board draws it: a solid color by meaning, an icon, the title and an optional
// line under it, and a 44px close button. Sonner's list around it is the live region that
// announces it.
// It takes pointer events itself (pointer-events-auto): an open sheet or dialog turns them off
// on the page, and the close button and the swipe must still work over it.
export function AppToast({ id, tone, title, description }: AppToastProps) {
  const { className, icon: Icon } = TOAST_TONES[tone];
  return (
    <div
      data-tone={tone}
      className={cn(
        "pointer-events-auto flex w-full items-start gap-3 rounded-2xl py-3.5 pr-2 pl-4 shadow-[0_8px_24px_rgba(30,42,43,0.22)] md:w-[420px]",
        className,
      )}
    >
      <Icon aria-hidden="true" className="mt-0.5 size-[22px] shrink-0" />
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <p className="text-base leading-snug font-bold">{title}</p>
        {description !== null && (
          <p className="text-[15px] leading-snug opacity-95">{description}</p>
        )}
      </div>
      <button
        type="button"
        aria-label={TOAST_CLOSE_LABEL}
        onClick={() => toast.dismiss(id)}
        className="-my-2.5 flex size-11 shrink-0 items-center justify-center rounded-lg outline-none hover:bg-white/15 focus-visible:ring-[3px] focus-visible:ring-current"
      >
        <XIcon aria-hidden="true" className="size-[18px]" />
      </button>
    </div>
  );
}
