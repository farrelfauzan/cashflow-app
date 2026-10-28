import { Suspense } from "react";

import { LogoutButton } from "@/components/client/auth/logout-button";
import { Card } from "@/components/ui/card";

import { OrganizationList } from "./organization-list";

// Temporary: proves server-side API calls. The organization picker comes in W4 from its
// approved board. The heading is static and prerenders; the list reads the session cookie, so
// it streams in behind the boundary.
export function OrganizationsPlaceholder() {
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md gap-4 px-6 py-8">
        <h1 className="text-2xl font-semibold tracking-tight">Organisasi</h1>
        <Suspense
          fallback={<p className="text-base text-muted-foreground">Memuat…</p>}
        >
          <OrganizationList />
        </Suspense>
        {/* Until the app shell (W4) has a place for it. */}
        <LogoutButton />
      </Card>
    </main>
  );
}
