"use client";

import { Toaster } from "@/components/ui/sonner";

import { TOASTER_LABEL } from "./utils";

// The one place toasts appear, at the top center (board "Notifikasi singkat"): 16px under the
// top on iPad and desktop, 12px under the phone's safe area, at most three stacked. Swipe up on a
// phone to dismiss. Mounted once in the staff layout; tenant pages have none.
export function AppToaster() {
  return (
    <Toaster
      position="top-center"
      visibleToasts={3}
      offset={{ top: 16 }}
      mobileOffset={{
        top: "calc(env(safe-area-inset-top) + 12px)",
        left: 12,
        right: 12,
      }}
      containerAriaLabel={TOASTER_LABEL}
      toastOptions={{ unstyled: true }}
    />
  );
}
