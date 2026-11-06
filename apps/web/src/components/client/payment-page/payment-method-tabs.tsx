"use client";

import type { ReactNode } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import type { PaymentTab } from "./type";
import {
  PAYMENT_TABS_LABEL,
  QRIS_TAB_TEXT,
  TRANSFER_TAB_TEXT,
  toPaymentTab,
} from "./utils";

type PaymentMethodTabsProps = {
  tab: PaymentTab;
  onTabChange: (tab: PaymentTab) => void;
  qris: ReactNode;
  transfer: ReactNode;
};

// "Cara bayar": QRIS | Transfer bank. Radix Tabs gives the tablist its roles, aria-selected,
// aria-controls and arrow-key switching; each trigger is at least 44px tall.
export function PaymentMethodTabs({
  tab,
  onTabChange,
  qris,
  transfer,
}: PaymentMethodTabsProps) {
  return (
    <Tabs
      value={tab}
      onValueChange={(value) => onTabChange(toPaymentTab(value))}
      className="gap-3 md:gap-4"
    >
      <TabsList aria-label={PAYMENT_TABS_LABEL} className="w-full">
        <TabsTrigger
          value="qris"
          className="text-sm font-semibold md:text-base"
        >
          {QRIS_TAB_TEXT}
        </TabsTrigger>
        <TabsTrigger
          value="transfer"
          className="text-sm font-semibold md:text-base"
        >
          {TRANSFER_TAB_TEXT}
        </TabsTrigger>
      </TabsList>
      <TabsContent value="qris" className="flex flex-col gap-3 md:gap-4">
        {qris}
      </TabsContent>
      <TabsContent value="transfer" className="flex flex-col gap-3 md:gap-4">
        {transfer}
      </TabsContent>
    </Tabs>
  );
}
