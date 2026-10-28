import { Suspense } from "react";

import { OrganizationPickerContent } from "./organization-picker-content";

// Pilih usaha (boards Organizations.dc.html and Organizations-Phone.dc.html). Everything on it
// depends on who is signed in, so it streams in behind one boundary.
export function OrganizationPicker() {
  return (
    <main className="flex min-h-dvh flex-col gap-7 bg-background px-5 pt-5 pb-6 md:px-10 md:pt-7 md:pb-10">
      <Suspense
        fallback={<p className="text-base text-muted-foreground">Memuat…</p>}
      >
        <OrganizationPickerContent />
      </Suspense>
    </main>
  );
}
