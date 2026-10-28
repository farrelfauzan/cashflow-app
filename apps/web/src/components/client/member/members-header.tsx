"use client";

import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";

type MembersHeaderProps = {
  organizationName: string;
  // member:invite (allow or conditional): the screen's one primary action.
  canInvite: boolean;
  onInvite: () => void;
  onShowRoles: () => void;
};

// The title, what the screen is for, "Arti tiap peran", and "Tambah anggota". On iPad the two
// buttons sit to the right of the title; on the phone "Arti tiap peran" is a link in the text
// and "Tambah anggota" a full-width button under it.
export function MembersHeader({
  organizationName,
  canInvite,
  onInvite,
  onShowRoles,
}: MembersHeaderProps) {
  return (
    <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between lg:gap-5">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          Anggota tim
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          <span className="hidden lg:inline">
            {organizationName === ""
              ? "Siapa saja yang bisa masuk, dan apa yang boleh mereka lakukan."
              : `Siapa saja yang bisa masuk ke ${organizationName}, dan apa yang boleh mereka lakukan.`}
          </span>
          <span className="lg:hidden">
            Siapa saja yang bisa masuk, dan apa yang boleh mereka lakukan.{" "}
          </span>
          {/* Padding keeps a 44px touch target; the negative margin keeps the line height. */}
          <Button
            type="button"
            variant="link"
            onClick={onShowRoles}
            className="-my-3 inline h-auto px-0 py-3 text-[15px] font-semibold whitespace-normal lg:hidden"
          >
            Arti tiap peran
          </Button>
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <Button
          type="button"
          variant="ghost"
          onClick={onShowRoles}
          className="hidden h-12 px-3.5 font-semibold text-primary lg:inline-flex"
        >
          Arti tiap peran
        </Button>
        {canInvite && (
          <Button
            type="button"
            onClick={onInvite}
            className="h-13 w-full rounded-xl text-[17px] font-semibold lg:h-12 lg:w-auto"
          >
            <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
            Tambah anggota
          </Button>
        )}
      </div>
    </header>
  );
}
