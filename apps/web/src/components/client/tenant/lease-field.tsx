"use client";

import type { ReactNode } from "react";

type LeaseFieldProps = {
  label: string;
  children: ReactNode;
};

// One labelled value of a lease. On the phone the fields stack with a line between them; on iPad
// they sit in a grid.
export function LeaseField({ label, children }: LeaseFieldProps) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 border-t py-2.5 md:gap-[3px] md:border-t-0 md:py-0">
      <span className="text-sm text-muted-foreground md:text-base">
        {label}
      </span>
      <div className="text-base leading-[1.4] font-semibold md:text-[17px]">
        {children}
      </div>
    </div>
  );
}
