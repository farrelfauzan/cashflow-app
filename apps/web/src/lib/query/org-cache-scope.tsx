"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import { useEffect, useRef } from "react";

import { removeOrgQueries } from "./query-client";

// The org-switch rule: when the `orgId` route param changes (another organization, or leaving
// /o/[orgId] altogether), every query under ['org', <previous id>] is removed. Renders nothing.
export function OrgCacheScope() {
  const params = useParams<{ orgId?: string | string[] }>();
  const orgId = typeof params?.orgId === "string" ? params.orgId : undefined;
  const queryClient = useQueryClient();
  const previousOrgId = useRef(orgId);

  useEffect(() => {
    const previous = previousOrgId.current;
    if (previous !== undefined && previous !== orgId) {
      removeOrgQueries(queryClient, previous);
    }
    previousOrgId.current = orgId;
  }, [orgId, queryClient]);

  return null;
}
