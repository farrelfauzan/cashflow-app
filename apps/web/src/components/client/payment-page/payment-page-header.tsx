"use client";

import type { PaymentPageHeaderView } from "./type";

type PaymentPageHeaderProps = {
  header: PaymentPageHeaderView;
};

// Who bills the tenant, which invoice the link is for, and whose it is, so a forwarded link is
// recognizable.
export function PaymentPageHeader({ header }: PaymentPageHeaderProps) {
  return (
    <header className="flex flex-col gap-1">
      <p className="text-sm font-semibold text-primary md:text-base">
        {header.organizationName}
      </p>
      <h1 className="text-2xl leading-tight font-bold md:text-[28px]">
        {header.title}
      </h1>
      <p className="text-sm text-muted-foreground md:text-base">
        {header.subtitle}
      </p>
    </header>
  );
}
