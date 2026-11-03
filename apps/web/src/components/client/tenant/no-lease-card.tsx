"use client";

import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { noLeaseText } from "./utils";

type NoLeaseCardProps = {
  name: string;
  // lease:manage (allow or conditional): "Sewakan unit".
  canManageLeases: boolean;
  // "Catat pembayaran" is the page's primary action: "Sewakan unit" is then an outline button.
  secondary: boolean;
  onRent: () => void;
};

// "Belum ada sewa" (board Sewa-Baru.dc.html): a tenant without any lease the member may see, and
// the button that rents them a unit.
export function NoLeaseCard({
  name,
  canManageLeases,
  secondary,
  onRent,
}: NoLeaseCardProps) {
  return (
    <section
      aria-labelledby="no-lease"
      className="flex flex-col items-start gap-3.5 rounded-[14px] border bg-card p-4 md:rounded-2xl md:p-6"
    >
      <h2 id="no-lease" className="text-xl font-bold md:text-2xl">
        Belum ada sewa
      </h2>
      <p className="text-base leading-[1.55] text-muted-foreground md:text-[17px]">
        {noLeaseText(name, canManageLeases)}
      </p>
      {canManageLeases && (
        <Button
          type="button"
          variant={secondary ? "outline" : "default"}
          onClick={onRent}
          className={cn(
            "h-12 w-full rounded-xl px-5 text-[17px] font-semibold md:w-auto",
            secondary && "border-[1.5px] bg-card",
          )}
        >
          <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
          Sewakan unit
        </Button>
      )}
    </section>
  );
}
