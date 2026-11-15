"use client";

import type { ReactNode } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import type { DepositTab } from "./type";
import {
  CONFIRMED_TAB_TEXT,
  PENDING_TAB_TEXT,
  TABS_LABEL,
  toDepositTab,
} from "./utils";

import { CountBadge } from "./count-badge";

type DepositTabsProps = {
  tab: DepositTab;
  // The pending list's total, once the API answered.
  pendingCount: number | undefined;
  onTabChange: (tab: DepositTab) => void;
  // Only the chosen tab's content is rendered by the caller.
  children: ReactNode;
};

// "Menunggu dihitung [N]" | "Sudah dihitung". Radix Tabs gives the roles, aria-selected and arrow
// keys; each trigger is at least 44px tall.
export function DepositTabs({
  tab,
  pendingCount,
  onTabChange,
  children,
}: DepositTabsProps) {
  const showCount = pendingCount !== undefined && pendingCount > 0;
  return (
    <Tabs
      value={tab}
      onValueChange={(value) => onTabChange(toDepositTab(value))}
      className="gap-4"
    >
      <TabsList aria-label={TABS_LABEL} className="w-full md:w-fit">
        <TabsTrigger
          value="pending"
          className="px-3 text-sm font-semibold md:px-4 md:text-base"
          aria-label={
            showCount
              ? `${PENDING_TAB_TEXT}, ${pendingCount}`
              : PENDING_TAB_TEXT
          }
        >
          {PENDING_TAB_TEXT}
          {showCount && <CountBadge count={pendingCount} />}
        </TabsTrigger>
        <TabsTrigger
          value="confirmed"
          className="px-3 text-sm font-semibold md:px-4 md:text-base"
        >
          {CONFIRMED_TAB_TEXT}
        </TabsTrigger>
      </TabsList>
      <TabsContent value={tab} className="flex flex-col gap-4">
        {children}
      </TabsContent>
    </Tabs>
  );
}
