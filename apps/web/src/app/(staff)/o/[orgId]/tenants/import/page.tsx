import { TenantImportScreen } from "@/components/server/tenant-import/tenant-import-screen";

export default function TenantImportPage({
  params,
}: PageProps<"/o/[orgId]/tenants/import">) {
  return <TenantImportScreen params={params} />;
}
