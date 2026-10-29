"use client";

import type { ReactNode } from "react";

type ImportPhoneFooterProps = {
  children: ReactNode;
};

// The phone's bar at the bottom of the screen that holds the step's primary action (boards
// Impor*-Phone.dc.html). Hidden on iPad, where the action sits in the page.
export function ImportPhoneFooter({ children }: ImportPhoneFooterProps) {
  return (
    <div className="sticky bottom-0 z-10 -mx-4 -mb-6 flex flex-col gap-2 border-t bg-card px-4 py-3 lg:hidden">
      {children}
    </div>
  );
}
