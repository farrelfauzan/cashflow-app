"use client";

import { useState, type ReactNode } from "react";

import { cn } from "@/lib/utils";

import { SidebarCollapsedContext } from "./sidebar-collapsed-context";
import { SidebarToggle } from "./sidebar-toggle";
import { markSidebarChoice, sidebarCookie, SIDEBAR_COLLAPSED } from "./utils";

type SidebarRailProps = {
  // From the cf_sidebar cookie, read by the server shell, so the first paint is right.
  defaultCollapsed: boolean;
  // The menu content (OrgNav), rendered on the server.
  children: ReactNode;
};

// The iPad side menu: wide with icons and labels, or collapsed to a rail of icons to give the
// page more room. The toggle at its foot switches it; the choice is kept in a cookie, so it
// survives navigation and reloads. Hidden on the phone, which has its own menu sheet. On iPadOS
// it floats as a rounded glass panel 12px from the edges (board iOS-iPad, T-097e).
export function SidebarRail({ defaultCollapsed, children }: SidebarRailProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  const toggle = () => {
    const next = !collapsed;
    setCollapsed(next);
    document.cookie = sidebarCookie(
      next,
      window.location.protocol === "https:",
    );
    markSidebarChoice(document.documentElement, next);
  };

  return (
    <SidebarCollapsedContext.Provider value={collapsed}>
      <aside
        id="org-sidebar"
        data-sidebar={collapsed ? SIDEBAR_COLLAPSED : undefined}
        data-glass="chrome"
        className={cn(
          "sticky top-0 hidden h-dvh shrink-0 flex-col border-r bg-secondary/60 pl-[env(safe-area-inset-left)] transition-[width] duration-200 motion-reduce:transition-none md:flex ios:top-3 ios:my-3 ios:ml-[calc(12px+env(safe-area-inset-left))] ios:h-[calc(100dvh-24px)] ios:overflow-hidden ios:rounded-3xl ios:border ios:bg-card ios:pl-0",
          collapsed
            ? "w-[calc(76px+env(safe-area-inset-left))] ios:w-[76px]"
            : "w-[calc(228px+env(safe-area-inset-left))] ios:w-[228px]",
        )}
      >
        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
        <SidebarToggle collapsed={collapsed} onToggle={toggle} />
      </aside>
    </SidebarCollapsedContext.Provider>
  );
}
