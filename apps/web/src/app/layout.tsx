import type { Metadata } from "next";

import { SIDEBAR_CHOICE_SCRIPT } from "@/components/client/shell/utils";
import { APP_NAME } from "@/lib/app-name";
import { QueryProvider } from "@/lib/query/query-provider";

import { plusJakartaSans } from "./utils";

import "./globals.css";

export const metadata: Metadata = {
  title: APP_NAME,
  description: "Tagihan dan pembukuan sewa properti.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    // The inline script marks <html> with the side menu's choice before the first paint, hence
    // suppressHydrationWarning (it reaches only <html>'s own attributes).
    // view-transition-name "root" is the browser's default, written out: when a navigation's
    // changes look to React as if they were all inside a <ViewTransition>, React takes the screen
    // out of the view transition unless <html> already names it, and the phone's push would then
    // lose its incoming page (PageMotion, board HP-Pindah).
    <html
      lang="id"
      className={plusJakartaSans.variable}
      style={{ viewTransitionName: "root" }}
      suppressHydrationWarning
    >
      <head>
        <script dangerouslySetInnerHTML={{ __html: SIDEBAR_CHOICE_SCRIPT }} />
      </head>
      <body>
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
