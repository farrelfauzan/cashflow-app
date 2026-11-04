// What shows while the shell loads: the cream page and the side menu's surface on iPad, wide or
// collapsed to its rail as the member left it. This is prerendered without the request's
// cookies, so the width follows the mark the root layout's script puts on <html>
// (SIDEBAR_CHOICE_SCRIPT), which the toggle keeps up to date.
export function OrgShellSkeleton() {
  return (
    <div className="flex min-h-dvh bg-background">
      <div className="hidden w-[228px] shrink-0 border-r bg-secondary/60 md:block rail-choice:w-[76px]" />
      <p className="px-4 py-5 text-base text-muted-foreground md:px-8 md:py-8">
        Memuat…
      </p>
    </div>
  );
}
