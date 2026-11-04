import type { Metadata } from "next";
import { Suspense } from "react";

import { PaymentPageLoading } from "@/components/client/payment-page/payment-page-loading";
import { PaymentPageScreen } from "@/components/client/payment-page/payment-page-screen";

// The tenant payment page. Public: the token in the path is the access, so it stays outside
// src/proxy.ts's matcher and the (staff) group (no PWA manifest). The title names nobody and
// never the token; search engines skip it, and no request from it carries its URL as a
// referrer (Caddy sends the same Referrer-Policy header on /t/*).
export const metadata: Metadata = {
  title: "Bayar tagihan",
  robots: { index: false, follow: false },
  referrer: "no-referrer",
};

// The data is read in the browser (PaymentPageScreen), never here.
export default function TenantPaymentPage({ params }: PageProps<"/t/[token]">) {
  return (
    <Suspense fallback={<PaymentPageLoading />}>
      <PaymentPageScreen params={params} />
    </Suspense>
  );
}
