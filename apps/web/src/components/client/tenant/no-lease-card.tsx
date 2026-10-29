"use client";

import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { noLeaseText } from "./utils";

type NoLeaseCardProps = {
  name: string;
  // lease:manage (allow or conditional): "Sewakan unit".
  canManageLeases: boolean;
  onRent: () => void;
};

// "Belum ada sewa" (board Sewa-Baru.dc.html): a tenant without any lease the member may see, and
// the button that rents them a unit.
export function NoLeaseCard({
  name,
  canManageLeases,
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
          onClick={onRent}
          className="h-12 w-full rounded-xl px-5 text-[17px] font-semibold md:w-auto"
        >
          <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
          Sewakan unit
        </Button>
      )}
    </section>
  );
}
