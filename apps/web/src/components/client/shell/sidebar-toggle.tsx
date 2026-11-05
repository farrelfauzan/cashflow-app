"use client";

import { PanelLeftClose, PanelLeftOpen } from "lucide-react";

import { Button } from "@/components/ui/button";

import { sidebarToggleLabel } from "./utils";

type SidebarToggleProps = {
  collapsed: boolean;
  onToggle: () => void;
};

// "Ciutkan menu" / "Lebarkan menu" at the foot of the iPad side menu. On the rail only the icon
// shows; the text stays as its accessible name and its hover title.
export function SidebarToggle({ collapsed, onToggle }: SidebarToggleProps) {
  const label = sidebarToggleLabel(collapsed);
  const Icon = collapsed ? PanelLeftOpen : PanelLeftClose;

  return (
    <div className="border-t px-3 py-2">
      <Button
        type="button"
        variant="ghost"
        aria-expanded={!collapsed}
        aria-controls="org-sidebar"
        title={collapsed ? label : undefined}
        onClick={onToggle}
        className="h-11 w-full justify-start gap-3 px-3 text-base font-semibold text-muted-foreground rail:justify-center rail:px-0"
      >
        <Icon aria-hidden="true" className="size-5" />
        <span className="rail:sr-only">{label}</span>
      </Button>
    </div>
  );
}
