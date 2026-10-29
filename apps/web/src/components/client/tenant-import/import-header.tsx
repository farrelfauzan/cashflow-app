"use client";

import { ChevronLeft } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { orgPath } from "@/lib/org-path";

type ImportHeaderProps = {
  orgId: string;
  title: string;
  subtitle: string;
  // The step's own header buttons (step 2), to the right of the title on iPad.
  actions?: ReactNode;
};

// The back link "Penyewa", the step's title and what it means (boards Impor*.dc.html).
export function ImportHeader({
  orgId,
  title,
  subtitle,
  actions,
}: ImportHeaderProps) {
  return (
    <header className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between lg:gap-5">
      <div className="flex min-w-0 flex-col gap-1.5">
        <Link
          href={orgPath(orgId, "tenants")}
          className="-ml-1 inline-flex min-h-11 items-center gap-1 self-start rounded-md pr-2 text-[15px] font-semibold text-primary hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:text-base"
        >
          <ChevronLeft
            aria-hidden="true"
            className="size-[18px]"
            strokeWidth={2.2}
          />
          Penyewa
        </Link>
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          {title}
        </h1>
        <p className="text-[15px] leading-normal break-words text-muted-foreground md:text-[17px]">
          {subtitle}
        </p>
      </div>
      {actions}
    </header>
  );
}
