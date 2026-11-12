"use client";

import Link from "next/link";

import { FORWARD_TYPES } from "@/components/client/page-motion/utils";

import { TENANT_LINK_LABEL } from "./utils";

type MeterTenantLinkProps = {
  // Detail penyewa, where "Catat meteran" is.
  href: string;
};

// "Lihat penyewa": the way from a row to Detail penyewa, 44px tall.
export function MeterTenantLink({ href }: MeterTenantLinkProps) {
  return (
    <Link
      transitionTypes={FORWARD_TYPES}
      href={href}
      className="inline-flex min-h-11 items-center text-[15px] font-semibold text-primary underline-offset-4 hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:text-base"
    >
      {TENANT_LINK_LABEL}
    </Link>
  );
}
