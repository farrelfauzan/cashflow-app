import type { ReactNode } from "react";

import { MobileMenu } from "@/components/client/shell/mobile-menu";

type MobileHeaderProps = {
  organizationName: string;
  // "Pak Budi Hartono · Admin"
  detail: string;
  // The menu content (OrgNav), shown in a sheet.
  children: ReactNode;
};

// The phone's top bar (board Members-Phone.dc.html): the menu button, the organization, and who
// is signed in. Hidden on iPad, where the side menu is always open.
export function MobileHeader({
  organizationName,
  detail,
  children,
}: MobileHeaderProps) {
  return (
    <header className="sticky top-0 z-10 flex items-center gap-2 border-b bg-secondary px-3 py-2.5 md:hidden">
      <MobileMenu>{children}</MobileMenu>
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-[15px] font-bold">
          {organizationName}
        </span>
        <span className="truncate text-sm text-muted-foreground">{detail}</span>
      </div>
    </header>
  );
}
