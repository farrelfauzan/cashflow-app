"use client";

import { Plus, Users } from "lucide-react";

import { Button } from "@/components/ui/button";

type TenantsEmptyProps = {
  // tenant:manage for the whole organization. The header hides its button meanwhile, so the
  // screen keeps one primary action.
  canAdd: boolean;
  onAdd: () => void;
};

// The organization has no tenant the member can see. The board's import button and its mention
// come with the import screen.
export function TenantsEmpty({ canAdd, onAdd }: TenantsEmptyProps) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-[14px] border bg-card px-5 py-8 text-center md:rounded-2xl md:px-7 md:py-11">
      <span
        aria-hidden="true"
        className="hidden size-16 items-center justify-center rounded-[18px] bg-secondary md:flex"
      >
        <Users className="size-8 text-muted-foreground" strokeWidth={1.8} />
      </span>
      <h2 className="text-[22px] font-bold md:text-2xl">Belum ada penyewa</h2>
      {canAdd && (
        <>
          <p className="max-w-[460px] text-base leading-[1.55] text-muted-foreground md:text-[17px]">
            Tambahkan penyewa satu per satu.
          </p>
          <Button
            type="button"
            onClick={onAdd}
            className="h-12 rounded-xl px-5 text-[17px] font-semibold"
          >
            <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
            Tambah penyewa
          </Button>
        </>
      )}
    </div>
  );
}
