"use client";

import { cn } from "cn";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

type OrgNavLinkProps = {
  href: string;
  // The home matches only itself; a section also matches the pages below it.
  exact: boolean;
  children: ReactNode;
};

// A side menu entry; the current page is marked for screen readers and drawn as a raised tile.
export function OrgNavLink({ href, exact, children }: OrgNavLinkProps) {
  const pathname = usePathname();
  const current =
    pathname === href || (!exact && pathname.startsWith(`${href}/`));

  return (
    <Link
      href={href}
      aria-current={current ? "page" : undefined}
      className={cn(
        "flex min-h-[46px] items-center gap-3 rounded-[10px] px-3 text-base text-foreground/85 hover:bg-accent/60",
        current && "bg-card font-semibold text-foreground shadow-xs",
      )}
    >
      {children}
    </Link>
  );
}
