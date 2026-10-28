import { PropertiesScreen } from "@/components/server/property/properties-screen";

export default function PropertiesPage({
  params,
}: PageProps<"/o/[orgId]/properties">) {
  return <PropertiesScreen params={params} />;
}
