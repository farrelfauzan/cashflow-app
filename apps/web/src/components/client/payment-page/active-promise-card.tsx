"use client";

import { CalendarCheck } from "lucide-react";

import { Button } from "@/components/ui/button";

import { EDIT_PROMISE_TEXT } from "./utils";

type ActivePromiseCardProps = {
  // "Janji pelunasan 1 Des 2026 · Semua sisa tagihan".
  text: string;
  // "Ubah janji"; null when no promise can be made now.
  onEdit: (() => void) | null;
};

// The tenant's active promise above the choice, for a tenant who opens the link from a reminder.
// The choices stay under it, so they can still pay.
export function ActivePromiseCard({ text, onEdit }: ActivePromiseCardProps) {
  return (
    <section className="flex flex-col gap-2.5 rounded-xl border bg-card px-4 py-3.5 shadow-xs md:px-5 md:py-4">
      <p className="flex items-start gap-2.5 text-sm font-semibold md:text-base">
        <CalendarCheck
          aria-hidden="true"
          className="mt-0.5 size-5 shrink-0 text-primary"
        />
        {text}
      </p>
      {onEdit !== null && (
        <Button
          type="button"
          variant="outline"
          className="self-start font-semibold"
          onClick={onEdit}
        >
          {EDIT_PROMISE_TEXT}
        </Button>
      )}
    </section>
  );
}
