import { LoginFlow } from "@/components/client/login/login-flow";
import { postLoginPath } from "@/lib/auth/post-login-path";

import { LoginTenantNote } from "./login-tenant-note";

type LoginFormSectionProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

// Reads `?next=` at request time (behind the screen's Suspense boundary, so the rest of the
// page stays in the static shell) and hands the client only the checked destination.
export async function LoginFormSection({
  searchParams,
}: LoginFormSectionProps) {
  const { next } = await searchParams;
  const redirectTo = postLoginPath(typeof next === "string" ? next : null);

  return <LoginFlow redirectTo={redirectTo} tenantNote={<LoginTenantNote />} />;
}
