"use client";

import { Info, TriangleAlert } from "lucide-react";

import { cn } from "@/lib/utils";

import type { CheckCalloutTone } from "./type";

type CheckCalloutProps = {
  tone: CheckCalloutTone;
  text: string;
};

// A note box in a drawer: the mint one for what did not change, the warning one for a danger.
export function CheckCallout({ tone, text }: CheckCalloutProps) {
  const Icon = tone === "warning" ? TriangleAlert : Info;
  return (
    <div
      className={cn(
        "flex items-start gap-2.5 rounded-xl px-3.5 py-3 text-foreground",
        tone === "warning"
          ? "border border-warning bg-warning/15"
          : "bg-accent",
      )}
    >
      <Icon
        aria-hidden="true"
        className={cn(
          "mt-0.5 size-5 shrink-0",
          tone === "warning" ? "text-warning" : "text-primary",
        )}
      />
      <p className="text-[15px] leading-normal md:text-base">{text}</p>
    </div>
  );
}
