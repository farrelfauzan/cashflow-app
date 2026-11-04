"use client";

import { createContext, useContext } from "react";

// Whether the iPad side menu is collapsed to its rail, for the client components inside it that
// change more than their styling (a hover title on an icon-only link). Outside the side menu,
// in the phone's menu sheet, it is false. Styling alone uses the `rail:` variant instead.
export const SidebarCollapsedContext = createContext(false);

export function useSidebarCollapsed(): boolean {
  return useContext(SidebarCollapsedContext);
}
