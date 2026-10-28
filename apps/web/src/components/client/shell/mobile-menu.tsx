"use client";

import { Menu } from "lucide-react";
import { usePathname } from "next/navigation";
import { useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";

type MobileMenuProps = {
  // The menu content, rendered on the server.
  children: ReactNode;
};

// The phone's menu: a button that opens the side menu as a sheet from the left. Following a
// link closes it.
export function MobileMenu({ children }: MobileMenuProps) {
  const pathname = usePathname();
  // The path the sheet was opened on: once a link changes the path, the sheet is closed.
  const [openOn, setOpenOn] = useState<string | null>(null);

  return (
    <Sheet
      open={openOn === pathname}
      onOpenChange={(open) => setOpenOn(open ? pathname : null)}
    >
      <SheetTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Buka menu">
          <Menu aria-hidden="true" className="size-[22px]" />
        </Button>
      </SheetTrigger>
      <SheetContent side="left" className="w-[300px] gap-0 bg-secondary p-0">
        <SheetTitle className="sr-only">Menu</SheetTitle>
        <SheetDescription className="sr-only">
          Pindah halaman, ganti usaha, atau keluar.
        </SheetDescription>
        {children}
      </SheetContent>
    </Sheet>
  );
}
