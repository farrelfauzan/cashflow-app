import { redirect } from "next/navigation";

import { DashboardView } from "@/components/client/dashboard/dashboard-view";
import { landingSegment, loadOrgMe } from "@/components/server/shell/utils";
import { createAppAbility } from "@/lib/capabilities";
import { orgPath } from "@/lib/org-path";

type DashboardScreenProps = {
  params: Promise<{ orgId: string }>;
};

// Beranda, the organization's home (board "iPad · Beranda Supervisor"). The figures load in the
// client view, from the member's capabilities and getDashboard.
//
// Every way into an organization (signing in, the picker, "Ganti usaha") opens /o/<orgId>, so this
// is the one place that sends a member who may not read reports on to their first section (board
// HP-Tab-Karyawan: a karyawan lands on Tagihan hari ini). The member's capabilities come from the
// same getOrgMe call as the shell's (loadOrgMe is cached per request); the shell sends anyone who
// is not a member back to the picker.
export async function DashboardScreen({ params }: DashboardScreenProps) {
  const { orgId } = await params;
  const orgMe = await loadOrgMe(orgId);
  if (orgMe !== null) {
    const segment = landingSegment(createAppAbility(orgMe.capabilities));
    if (segment !== null) redirect(orgPath(orgId, segment));
  }

  return <DashboardView orgId={orgId} />;
}
