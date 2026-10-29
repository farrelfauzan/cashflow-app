"use client";

import { Download } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { TEMPLATE_FILE_NAME, templateDownloadHref } from "./utils";

type TemplateDownloadLinkProps = {
  orgId: string;
  label: string;
  // "outline" in the format card, "ghost" (a text button) in the check's header.
  variant: "outline" | "ghost";
  className?: string;
};

// "Unduh template CSV". A plain same-origin link, not next/link (it is an API path) and not a
// client call: the API client takes only JSON answers (src/lib/api/client.ts, "Non-JSON
// downloads (the CSV template) are links, not client calls"). The browser saves the bytes as
// sent, byte order mark included, and the HttpOnly session cookie goes along by itself.
export function TemplateDownloadLink({
  orgId,
  label,
  variant,
  className,
}: TemplateDownloadLinkProps) {
  return (
    <Button
      asChild
      variant={variant}
      className={cn(
        "h-12 rounded-xl text-base font-semibold",
        variant === "outline"
          ? "border-[1.5px] bg-card px-[18px]"
          : "px-3 text-primary",
        className,
      )}
    >
      <a href={templateDownloadHref(orgId)} download={TEMPLATE_FILE_NAME}>
        <Download aria-hidden="true" className="size-5" strokeWidth={2} />
        {label}
      </a>
    </Button>
  );
}
