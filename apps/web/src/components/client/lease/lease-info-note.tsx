"use client";

import { Info } from "lucide-react";

type LeaseInfoNoteProps = {
  text: string;
};

// A grey note in the lease form: what a start date in the past or the future means.
export function LeaseInfoNote({ text }: LeaseInfoNoteProps) {
  return (
    <div className="flex items-start gap-2.5 rounded-xl bg-muted px-3.5 py-3 text-foreground">
      <Info aria-hidden="true" className="mt-0.5 size-5 shrink-0" />
      <p className="text-sm leading-normal md:text-base">{text}</p>
    </div>
  );
}
