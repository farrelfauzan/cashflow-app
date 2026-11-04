import { SIDEBAR_SKELETON_SCRIPT } from "@/components/client/shell/utils";

// What shows while the shell loads: the cream page and the side menu's surface on iPad, wide or
// collapsed to its rail as the member left it. This is prerendered without the request's
// cookies, so the inline script right after the surface marks it collapsed before it is painted
// (the attribute it adds is expected, hence suppressHydrationWarning).
export function OrgShellSkeleton() {
  return (
    <div className="flex min-h-dvh bg-background">
      <div
        suppressHydrationWarning
        className="hidden w-[228px] shrink-0 border-r bg-secondary/60 data-[sidebar=collapsed]:w-[76px] md:block"
      />
      <script dangerouslySetInnerHTML={{ __html: SIDEBAR_SKELETON_SCRIPT }} />
      <p className="px-4 py-5 text-base text-muted-foreground md:px-8 md:py-8">
        Memuat…
      </p>
    </div>
  );
}
