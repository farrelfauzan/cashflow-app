import type { Viewport } from "next";

import { OrgShell } from "@/components/server/shell/org-shell";

// Inside an organization the page runs under the notch and the home indicator ("cover"): the
// phone's header and tab bar pad themselves by the safe area (env(safe-area-inset-*)), which is
// 0 without it. Login and the organization picker keep the default, as they have no such padding.
// Merged with the staff layout's viewport, so its themeColor stays.
export const viewport: Viewport = {
  viewportFit: "cover",
};

export default function OrgLayout({
  children,
  params,
}: LayoutProps<"/o/[orgId]">) {
  return <OrgShell params={params}>{children}</OrgShell>;
}
