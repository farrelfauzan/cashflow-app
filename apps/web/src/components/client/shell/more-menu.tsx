"use client";

import { cn } from "cn";
import { Ellipsis } from "lucide-react";
import { usePathname } from "next/navigation";
import { useState, type ReactNode } from "react";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";

type MoreMenuProps = {
  label: string;
  // Pages that live in this menu (Anggota tim): the tab shows as current on them.
  sectionHrefs: readonly string[];
  // The menu's content, rendered on the server.
  children: ReactNode;
};

// "Lainnya", the last tab on the phone (board HP-Lainnya): opens a sheet from the bottom with the
// sections that are not tabs, "Ganti usaha", "Telegram" and "Keluar". Following a link closes it.
export function MoreMenu({ label, sectionHrefs, children }: MoreMenuProps) {
  const pathname = usePathname();
  // The path the sheet was opened on: once a link changes the path, the sheet is closed.
  const [openOn, setOpenOn] = useState<string | null>(null);
  const open = openOn === pathname;
  // A link to the page already open does not change the path, so any link click closes it too.
  const closeOnLink = ({ target }: { target: EventTarget }) => {
    if (target instanceof Element && target.closest("a")) setOpenOn(null);
  };
  const current = sectionHrefs.some(
    (href) => pathname === href || pathname.startsWith(`${href}/`),
  );

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => setOpenOn(next ? pathname : null)}
    >
      <SheetTrigger asChild>
        <button
          type="button"
          className={cn(
            "flex min-h-[58px] min-w-0 flex-1 flex-col items-center justify-center gap-[3px] rounded-xl text-[13px] font-semibold text-muted-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring",
            (open || current) && "font-bold text-primary-strong",
          )}
        >
          <span
            className={cn(
              "flex h-[30px] w-[52px] items-center justify-center rounded-full",
              (open || current) && "bg-accent text-primary",
            )}
          >
            <Ellipsis aria-hidden="true" className="size-[22px]" />
          </span>
          <span className="truncate">{label}</span>
        </button>
      </SheetTrigger>
      <SheetContent
        side="bottom"
        className="max-h-[92dvh] gap-0 rounded-t-[20px] pb-[env(safe-area-inset-bottom)]"
      >
        <div className="flex justify-center pt-2 pb-0.5">
          <span
            aria-hidden="true"
            className="h-[5px] w-10 rounded-full bg-border"
          />
        </div>
        <div className="flex min-h-14 items-center border-b pr-14 pl-5">
          <SheetTitle className="text-xl font-bold">{label}</SheetTitle>
        </div>
        <SheetDescription className="sr-only">
          Bagian lain, ganti usaha, atau keluar.
        </SheetDescription>
        <div
          className="overflow-y-auto px-5 pt-4 pb-5"
          onClickCapture={closeOnLink}
        >
          {children}
        </div>
      </SheetContent>
    </Sheet>
  );
}
