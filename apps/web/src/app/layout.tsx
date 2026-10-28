import type { Metadata } from "next";

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
    <html lang="id" className={plusJakartaSans.variable}>
      <body>
        <QueryProvider>{children}</QueryProvider>
      </body>
    </html>
  );
}
