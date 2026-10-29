"use client";

import { ChevronLeft } from "lucide-react";
import Link from "next/link";

import { orgPath } from "@/lib/org-path";
import { NO_ACCESS_TEXT } from "./utils";

type ImportNoAccessProps = {
  orgId: string;
};

// What a member without the import's capabilities sees when they open its address directly. The
// screen sends no request: the API would refuse the import anyway.
export function ImportNoAccess({ orgId }: ImportNoAccessProps) {
  return (
    <div className="flex flex-col items-start gap-3">
      <Link
        href={orgPath(orgId, "tenants")}
        className="-ml-1 inline-flex min-h-11 items-center gap-1 rounded-md pr-2 text-[15px] font-semibold text-primary hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:text-base"
      >
        <ChevronLeft
          aria-hidden="true"
          className="size-[18px]"
          strokeWidth={2.2}
        />
        Penyewa
      </Link>
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        Impor penyewa
      </h1>
      <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
        {NO_ACCESS_TEXT}
      </p>
    </div>
  );
}
