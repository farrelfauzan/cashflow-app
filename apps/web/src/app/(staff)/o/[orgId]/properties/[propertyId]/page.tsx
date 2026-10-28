import { PropertyUnitsScreen } from "@/components/server/property/property-units-screen";

export default function PropertyUnitsPage({
  params,
}: PageProps<"/o/[orgId]/properties/[propertyId]">) {
  return <PropertyUnitsScreen params={params} />;
}
