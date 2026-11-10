"use client";

import { Clock } from "lucide-react";
import Link from "next/link";

import { FORWARD_TYPES } from "@/components/client/page-motion/utils";
import { orgPath } from "@/lib/org-path";
import type { ArrearsRowView } from "./type";

type ArrearsRowProps = {
  orgId: string;
  row: ArrearsRowView;
  canOpenTenant: boolean;
};

// One tenant in arrears: name and unit, what they owe and since when. The name opens Detail
// penyewa for a member who may read tenants. Its tap box is 44px tall; the negative margin keeps
// that box from stretching the line, so a place that wraps keeps its normal line spacing.
export function ArrearsRow({ orgId, row, canOpenTenant }: ArrearsRowProps) {
  return (
    <li className="flex flex-col gap-1 border-t py-3 first:border-t-0">
      <p className="text-[17px] font-semibold md:text-xl">
        {canOpenTenant ? (
          <Link
            transitionTypes={FORWARD_TYPES}
            href={orgPath(orgId, `tenants/${row.tenantId}`)}
            className="-my-2.5 inline-flex min-h-11 items-center underline-offset-4 hover:underline"
          >
            {row.name}
          </Link>
        ) : (
          row.name
        )}
        {row.place !== "" && (
          <span className="font-normal text-muted-foreground">
            {" "}
            · {row.place}
          </span>
        )}
      </p>
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[15px] md:text-[17px]">
        <span className="font-semibold">{row.amount}</span>
        <span className="inline-flex items-center gap-1.5 font-semibold text-warning">
          <Clock aria-hidden="true" className="size-[18px]" />
          {row.age}
        </span>
      </p>
    </li>
  );
}
