import { OrgTile } from "@/components/server/organization/org-tile";

type MobileHeaderProps = {
  organizationName: string;
  // "Pak Budi Hartono · Admin"
  detail: string;
};

// The phone's top bar (board HP-Tab): the organization and who is signed in, under the status
// bar. Navigation is the tab bar at the bottom (PhoneTabBar). Hidden on iPad, where the side
// menu is always open.
export function MobileHeader({ organizationName, detail }: MobileHeaderProps) {
  return (
    <header className="sticky top-0 z-10 flex items-center gap-2.5 border-b bg-secondary px-4 pt-[calc(env(safe-area-inset-top)+10px)] pb-2.5 md:hidden">
      <OrgTile
        name={organizationName}
        tone="solid"
        className="size-9 shrink-0 rounded-[10px] text-sm"
      />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[15px] font-bold">
          {organizationName}
        </span>
        <span className="truncate text-sm text-muted-foreground">{detail}</span>
      </div>
    </header>
  );
}
