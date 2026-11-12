"use client";

import Link from "next/link";

import { FORWARD_TYPES } from "@/components/client/page-motion/utils";
import { cn } from "@/lib/utils";

type InvoiceTenantNameProps = {
  name: string;
  // Detail penyewa; null when the member may not read tenants, so the name is plain text.
  href: string | null;
  className?: string;
};

// The tenant's name on an invoice: a link to Detail penyewa that is 44px tall without making its
// line taller (the negative margin takes the extra height from the row's padding).
export function InvoiceTenantName({
  name,
  href,
  className,
}: InvoiceTenantNameProps) {
  if (href === null) {
    return <span className={className}>{name}</span>;
  }
  return (
    <Link
      transitionTypes={FORWARD_TYPES}
      href={href}
      className={cn(
        "-my-2.5 inline-flex min-h-11 items-center hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none",
        className,
      )}
    >
      {name}
    </Link>
  );
}
