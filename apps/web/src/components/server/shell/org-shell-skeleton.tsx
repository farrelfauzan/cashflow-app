// What shows while the shell loads: the cream page and the side menu's surface on iPad.
export function OrgShellSkeleton() {
  return (
    <div className="flex min-h-dvh bg-background">
      <div className="hidden w-[228px] shrink-0 border-r bg-secondary/60 md:block" />
      <p className="px-4 py-5 text-base text-muted-foreground md:px-8 md:py-8">
        Memuat…
      </p>
    </div>
  );
}
