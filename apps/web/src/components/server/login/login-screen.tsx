import { Suspense } from "react";

import { AppBrand } from "@/components/server/brand/app-brand";

import { LoginFormSection } from "./login-form-section";

type LoginScreenProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

// Staff login (boards Login.dc.html and Login-iPad.dc.html): full-bleed on phones, a centered
// card on iPad.
export function LoginScreen({ searchParams }: LoginScreenProps) {
  return (
    <main className="flex min-h-dvh flex-col gap-7 bg-background px-5 pt-7 pb-6 md:items-center md:justify-center md:gap-6 md:px-6 md:py-10">
      <AppBrand />
      <Suspense fallback={null}>
        <LoginFormSection searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
