"use client";

import { cn } from "cn";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

type PhoneTabLinkProps = {
  href: string;
  // The home matches only itself; a section also matches the pages below it.
  exact: boolean;
  label: string;
  // The section's icon, rendered on the server.
  icon: ReactNode;
};

// One tab of the phone's tab bar (board HP-Tab): the icon on a mint pill and a bold label when it
// is the current section, which is also marked for screen readers. 58px tall; 56px in the
// floating bar on iOS, where the other tabs are ink as on the board (iOS-HP-Tab). While pressed
// the mint pill shows behind any tab's icon (board HP-Tekan).
export function PhoneTabLink({ href, exact, label, icon }: PhoneTabLinkProps) {
  const pathname = usePathname();
  const current =
    pathname === href || (!exact && pathname.startsWith(`${href}/`));

  return (
    <Link
      href={href}
      aria-current={current ? "page" : undefined}
      className={cn(
        "group flex min-h-[58px] min-w-0 flex-1 flex-col items-center justify-center gap-[3px] rounded-xl text-[13px] font-semibold text-muted-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring ios:min-h-14 ios:gap-0.5 ios:rounded-[26px]",
        current ? "font-bold text-primary-strong" : "ios:text-foreground",
      )}
    >
      <span
        className={cn(
          "flex h-[30px] w-[52px] items-center justify-center rounded-full transition-colors duration-100 group-active:bg-accent ios:h-7 ios:w-[50px] [&_svg]:size-[22px]",
          current && "bg-accent text-primary",
        )}
      >
        {icon}
      </span>
      <span className="truncate">{label}</span>
    </Link>
  );
}
