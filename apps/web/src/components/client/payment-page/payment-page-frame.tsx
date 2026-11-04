"use client";

import type { ReactNode } from "react";

type PaymentPageFrameProps = {
  children: ReactNode;
};

// The page around every state: one centered column, phone first, at most 480px wide on iPad.
export function PaymentPageFrame({ children }: PaymentPageFrameProps) {
  return (
    <main className="mx-auto flex min-h-dvh w-full max-w-[480px] flex-col gap-4 px-5 pt-6 pb-8 md:gap-5 md:pt-12">
      {children}
    </main>
  );
}
