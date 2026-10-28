"use client";

import { AbilityProvider } from "@casl/react";
import { useLayoutEffect, useState, type ReactNode } from "react";

import { useGetOrgMe } from "@/lib/api/generated/members/members";
import type { Capability } from "@/lib/api/generated/model";
import { capabilityRules, createAppAbility } from "@/lib/capabilities";

import { currentCapabilities } from "./utils";

type OrgAbilityProviderProps = {
  orgId: string;
  // getOrgMe's capabilities, as the server loaded them with the page.
  capabilities: Capability[];
  children: ReactNode;
};

// What the member may do in this organization, for every client component under the shell
// (`useAbility`, `Can`). It starts from the server's capabilities, so controls show on the first
// paint, then follows getOrgMe. The shell keys it by organization, so a switch starts afresh and
// never shows another organization's controls; leaving the shell (logout, another organization)
// clears it.
export function OrgAbilityProvider({
  orgId,
  capabilities,
  children,
}: OrgAbilityProviderProps) {
  const orgMe = useGetOrgMe(orgId);
  const [ability] = useState(() => createAppAbility(capabilities));
  const current = currentCapabilities(
    capabilities,
    orgMe.data?.capabilities,
    orgMe.error,
  );

  // Before paint, so a control never shows for a frame on rules that no longer hold.
  useLayoutEffect(() => {
    ability.update(capabilityRules(current));
  }, [ability, current]);
  useLayoutEffect(
    () => () => {
      ability.update([]);
    },
    [ability],
  );

  return <AbilityProvider value={ability}>{children}</AbilityProvider>;
}
