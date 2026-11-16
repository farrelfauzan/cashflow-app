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
  // A count on the icon's top-right corner (board HP-Kunjungan), already capped ("99+").
  badge?: string;
  // The link's accessible name when the badge says something the label does not.
  accessibleName?: string;
};

// One tab of the phone's tab bar (board HP-Tab): the icon on a mint pill and a bold label when it
// is the current section, which is also marked for screen readers. 58px tall; 56px in the
// floating bar on iOS, where the other tabs are ink as on the board (iOS-HP-Tab). While pressed
// the mint pill shows behind any tab's icon (board HP-Tekan). The badge is a small circle in the
// accent colour, positioned over the icon's corner, so it changes neither the tab's size nor its
// layout; the accessible name carries its meaning, so the badge is hidden from readers. Its digits
// are drawn by CSS from data-count, so they never become part of the link's text.
export function PhoneTabLink({
  href,
  exact,
  label,
  icon,
  badge,
  accessibleName,
}: PhoneTabLinkProps) {
  const pathname = usePathname();
  const current =
    pathname === href || (!exact && pathname.startsWith(`${href}/`));

  return (
    <Link
      href={href}
      aria-current={current ? "page" : undefined}
      aria-label={accessibleName}
      className={cn(
        "group flex min-h-[58px] min-w-0 flex-1 flex-col items-center justify-center gap-[3px] rounded-xl text-[13px] font-semibold text-muted-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring ios:min-h-14 ios:gap-0.5 ios:rounded-[26px]",
        current ? "font-bold text-primary-strong" : "ios:text-foreground",
      )}
    >
      <span
        className={cn(
          "relative flex h-[30px] w-[52px] items-center justify-center rounded-full transition-colors duration-100 group-active:bg-accent ios:h-7 ios:w-[50px] [&_svg]:size-[22px]",
          current && "bg-accent text-primary",
        )}
      >
        {icon}
        {badge === undefined ? null : (
          <span
            aria-hidden="true"
            data-count={badge}
            className="absolute top-[-3px] right-[9px] flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-primary px-1 text-[11px] leading-none font-bold text-primary-foreground after:content-[attr(data-count)] ios:right-2"
          />
        )}
      </span>
      <span className="truncate">{label}</span>
    </Link>
  );
}
