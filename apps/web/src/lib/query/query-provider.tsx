"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { Suspense, type ReactNode } from "react";

import { OrgCacheScope } from "./org-cache-scope";
import { getQueryClient } from "./query-client";

type QueryProviderProps = {
  children: ReactNode;
};

export function QueryProvider({ children }: QueryProviderProps) {
  const queryClient = getQueryClient();
  return (
    <QueryClientProvider client={queryClient}>
      {/* Reads the route params; its own boundary keeps it from holding up the page. */}
      <Suspense fallback={null}>
        <OrgCacheScope />
      </Suspense>
      {children}
    </QueryClientProvider>
  );
}
