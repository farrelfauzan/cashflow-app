import type { Metadata } from "next";
import { Plus_Jakarta_Sans } from "next/font/google";

import { APP_NAME } from "@/lib/app-name";
import { QueryProvider } from "@/lib/query/query-provider";

import "./globals.css";

// Downloaded at build time and served from this app, so pages make no request to Google.
const plusJakartaSans = Plus_Jakarta_Sans({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-plus-jakarta-sans",
});

export const metadata: Metadata = {
  title: APP_NAME,
  description: "Tagihan dan pembukuan sewa properti.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="id" className={plusJakartaSans.variable}>
      <body>
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
