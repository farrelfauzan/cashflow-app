"use client";

import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";

type PropertiesHeaderProps = {
  organizationName: string;
  // property:manage for the whole organization: the screen's one primary action.
  canAdd: boolean;
  onAdd: () => void;
};

// The title, what the screen is for, and "Tambah properti". On iPad the button sits to the right
// of the title; on the phone it is a full-width button under it.
export function PropertiesHeader({
  organizationName,
  canAdd,
  onAdd,
}: PropertiesHeaderProps) {
  return (
    <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between lg:gap-5">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          Properti
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          <span className="hidden md:inline">
            {organizationName === ""
              ? "Gedung dan rumah yang disewakan."
              : `Gedung dan rumah yang disewakan di ${organizationName}.`}
          </span>
          <span className="md:hidden">Gedung dan rumah yang disewakan.</span>
        </p>
      </div>
      {canAdd && (
        <Button
          type="button"
          onClick={onAdd}
          className="h-13 w-full shrink-0 rounded-xl text-[17px] font-semibold lg:h-12 lg:w-auto"
        >
          <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
          Tambah properti
        </Button>
      )}
    </header>
  );
}
