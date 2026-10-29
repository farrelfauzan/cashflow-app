"use client";

import { useAbility } from "@casl/react";

import { tenantPermissions } from "@/components/client/tenant/utils";
import type { AppAbility } from "@/lib/capabilities";

import { ImportFlow } from "./import-flow";
import { ImportNoAccess } from "./import-no-access";

type TenantImportViewProps = {
  orgId: string;
  organizationName: string;
};

// The import screen, for members who may import (tenant:manage and lease:manage for the whole
// organization, from getOrgMe's capabilities). Anyone else gets a short notice and the screen
// sends nothing. Visibility only: the API checks the import itself.
export function TenantImportView({
  orgId,
  organizationName,
}: TenantImportViewProps) {
  const { canImport } = tenantPermissions(useAbility<AppAbility>());
  if (!canImport) return <ImportNoAccess orgId={orgId} />;
  return <ImportFlow orgId={orgId} organizationName={organizationName} />;
}
