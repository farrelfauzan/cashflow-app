import { MembersScreen } from "@/components/server/member/members-screen";

export default function MembersPage({
  params,
}: PageProps<"/o/[orgId]/members">) {
  return <MembersScreen params={params} />;
}
