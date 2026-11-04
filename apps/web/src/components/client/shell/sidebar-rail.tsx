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
// survives navigation and reloads. Hidden on the phone, which has its own menu sheet.
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
        className={cn(
          "sticky top-0 hidden h-dvh shrink-0 flex-col border-r bg-secondary/60 transition-[width] duration-200 motion-reduce:transition-none md:flex",
          collapsed ? "w-[76px]" : "w-[228px]",
        )}
      >
        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
        <SidebarToggle collapsed={collapsed} onToggle={toggle} />
      </aside>
    </SidebarCollapsedContext.Provider>
  );
}
