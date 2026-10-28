import { TenantDetailView } from "@/components/client/tenant/tenant-detail-view";

type TenantDetailScreenProps = {
  params: Promise<{ orgId: string; tenantId: string }>;
};

// "Penyewa dan sewanya" (boards Penyewa-Sewa.dc.html and Penyewa-Sewa-Phone.dc.html). The tenant
// loads in the client view, which says so when the API does not find it.
export async function TenantDetailScreen({ params }: TenantDetailScreenProps) {
  const { orgId, tenantId } = await params;

  return <TenantDetailView orgId={orgId} tenantId={tenantId} />;
}
