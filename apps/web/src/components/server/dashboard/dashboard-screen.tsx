import { DashboardView } from "@/components/client/dashboard/dashboard-view";

type DashboardScreenProps = {
  params: Promise<{ orgId: string }>;
};

// Beranda, the organization's home (board "iPad · Beranda Supervisor"). The figures load in the
// client view, from the member's capabilities and getDashboard.
export async function DashboardScreen({ params }: DashboardScreenProps) {
  const { orgId } = await params;

  return <DashboardView orgId={orgId} />;
}
