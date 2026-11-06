"use client";

import { ChevronDown } from "lucide-react";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

import {
  QRIS_GUIDE_APPS,
  QRIS_GUIDE_APPS_TITLE,
  QRIS_GUIDE_FALLBACK,
  QRIS_GUIDE_IPHONE_NOTE,
  QRIS_GUIDE_STEPS,
  QRIS_GUIDE_TITLE,
} from "./utils";

// "Cara bayar pakai QR yang diunduh": how to pay a live QR from the same phone, through the
// gallery of a bank or e-wallet app. Closed at first, so "Unduh QR" stays the one primary action.
export function QrisPayGuide() {
  return (
    <Collapsible className="group/guide rounded-xl border bg-card">
      <CollapsibleTrigger className="flex min-h-12 w-full items-center justify-between gap-3 rounded-xl px-4 py-3 text-left text-sm leading-normal font-semibold outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 md:px-5 md:text-base">
        {QRIS_GUIDE_TITLE}
        <ChevronDown
          aria-hidden="true"
          className="size-5 shrink-0 transition-transform group-data-[state=open]/guide:rotate-180"
        />
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-4 border-t px-4 pt-3 pb-4 text-sm leading-normal md:px-5 md:text-base">
        <ol className="flex list-decimal flex-col gap-2 pl-5">
          {QRIS_GUIDE_STEPS.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        <p className="text-muted-foreground">{QRIS_GUIDE_IPHONE_NOTE}</p>
        <div className="flex flex-col gap-2">
          <p className="font-semibold">{QRIS_GUIDE_APPS_TITLE}</p>
          <dl className="flex flex-col gap-2">
            {QRIS_GUIDE_APPS.map((app) => (
              <div key={app.name}>
                <dt className="font-semibold">{app.name}</dt>
                <dd>{app.steps}</dd>
              </div>
            ))}
          </dl>
        </div>
        <p>{QRIS_GUIDE_FALLBACK}</p>
      </CollapsibleContent>
    </Collapsible>
  );
}
