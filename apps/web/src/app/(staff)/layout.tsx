import type { Metadata, Viewport } from "next";

import { APP_NAME } from "@/lib/app-name";

// Staff pages only (login, the organization picker and everything under /o/): the PWA manifest
// and home-screen icon. Tenant pages (/t/...) sit outside this group, so they never link the
// manifest. There is no service worker: nothing is cached for offline use.
export const metadata: Metadata = {
  manifest: "/manifest.webmanifest",
  appleWebApp: { capable: true, title: APP_NAME, statusBarStyle: "default" },
  icons: { apple: "/apple-touch-icon.png" },
};

// The cream background, so the browser and status bar blend with the page.
export const viewport: Viewport = {
  themeColor: "#f5f3ee",
};

export default function StaffLayout({ children }: LayoutProps<"/">) {
  return children;
}
