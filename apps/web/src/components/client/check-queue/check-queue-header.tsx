"use client";

import { ChevronRight } from "lucide-react";

import { BackLink } from "@/components/client/page-motion/back-link";
import { orgPath } from "@/lib/org-path";

import { BILLING_LABEL, BREADCRUMB_LABEL, SUBTITLE, TITLE } from "./utils";

type CheckQueueHeaderProps = {
  orgId: string;
};

// "Penagihan › Antrean cek", the title and what the screen is for. Penagihan goes back to the
// invoices (the side menu keeps it active, as this page sits under it).
export function CheckQueueHeader({ orgId }: CheckQueueHeaderProps) {
  return (
    <header className="flex flex-col gap-1.5">
      <nav aria-label={BREADCRUMB_LABEL}>
        <ol className="flex items-center gap-1 text-[15px] md:text-base">
          <li>
            <BackLink
              href={orgPath(orgId, "billing")}
              className="-ml-1 inline-flex min-h-11 items-center rounded-md px-1 font-semibold text-primary transition-colors hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              {BILLING_LABEL}
            </BackLink>
          </li>
          <li aria-hidden="true" className="text-muted-foreground">
            <ChevronRight className="size-4" />
          </li>
          <li aria-current="page" className="text-muted-foreground">
            {TITLE}
          </li>
        </ol>
      </nav>
      <h1 className="-mt-1 text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        {TITLE}
      </h1>
      <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
        {SUBTITLE}
      </p>
    </header>
  );
}
