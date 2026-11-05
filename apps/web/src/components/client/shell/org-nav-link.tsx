"use client";

import { cn } from "cn";
import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { useSidebarCollapsed } from "./sidebar-collapsed-context";

type OrgNavLinkProps = {
  href: string;
  // The home matches only itself; a section also matches the pages below it.
  exact: boolean;
  // The section's name, the hover title of the icon-only link on the collapsed rail.
  label: string;
  children: ReactNode;
};

// A side menu entry; the current page is marked for screen readers and drawn as a raised tile.
export function OrgNavLink({ href, exact, label, children }: OrgNavLinkProps) {
  const pathname = usePathname();
  const collapsed = useSidebarCollapsed();
  const current =
    pathname === href || (!exact && pathname.startsWith(`${href}/`));

  return (
    <Link
      href={href}
      aria-current={current ? "page" : undefined}
      title={collapsed ? label : undefined}
      className={cn(
        "flex min-h-[46px] items-center gap-3 rounded-[10px] px-3 text-base text-foreground/85 hover:bg-accent/60 rail:justify-center rail:px-0",
        current && "bg-card font-semibold text-foreground shadow-xs",
      )}
    >
      {children}
    </Link>
  );
}
