import { OrgShell } from "@/components/server/shell/org-shell";

export default function OrgLayout({
  children,
  params,
}: LayoutProps<"/o/[orgId]">) {
  return <OrgShell params={params}>{children}</OrgShell>;
}
