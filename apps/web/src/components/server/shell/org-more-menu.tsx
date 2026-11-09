import { ArrowLeftRight, ChevronRight } from "lucide-react";
import Link from "next/link";

import { LogoutButton } from "@/components/client/auth/logout-button";
import { TelegramMenuItem } from "@/components/client/telegram/telegram-menu-item";

import type { OrgNavItem } from "./type";
import { ORG_NAV_ICONS, PICKER_PATH, SWITCH_ORG_LABEL } from "./utils";

type OrgMoreMenuProps = {
  userName: string;
  // "Admin di Griya Sejahtera".
  roleLine: string;
  // The sections that are not tabs (Anggota tim), only those the member may open.
  sections: readonly OrgNavItem[];
  // "Ganti usaha" shows only for a member of several organizations.
  canSwitch: boolean;
};

const ROW_CLASS =
  "flex min-h-14 items-center gap-3.5 border-b px-1 text-[17px] font-semibold text-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring [&>svg]:shrink-0";

// What "Lainnya" holds on the phone (board HP-Lainnya): who is signed in, the sections that are
// not tabs, "Ganti usaha" (to the organization picker), "Telegram" where the environment has a
// bot (decision 42), and "Keluar".
export function OrgMoreMenu({
  userName,
  roleLine,
  sections,
  canSwitch,
}: OrgMoreMenuProps) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-0.5">
        <span className="text-[17px] font-bold">{userName}</span>
        <span className="text-[15px] text-muted-foreground">{roleLine}</span>
      </div>
      <ul className="flex flex-col">
        {sections.map((section) => {
          const Icon = ORG_NAV_ICONS[section.key];
          return (
            <li key={section.key}>
              <Link href={section.href} className={ROW_CLASS}>
                <Icon
                  aria-hidden="true"
                  className="size-[22px] text-muted-foreground"
                />
                <span className="flex-1">{section.label}</span>
                <ChevronRight
                  aria-hidden="true"
                  className="size-5 text-muted-foreground"
                />
              </Link>
            </li>
          );
        })}
        {canSwitch && (
          <li>
            <Link href={PICKER_PATH} className={ROW_CLASS}>
              <ArrowLeftRight
                aria-hidden="true"
                className="size-[22px] text-muted-foreground"
              />
              <span className="flex-1">{SWITCH_ORG_LABEL}</span>
              <ChevronRight
                aria-hidden="true"
                className="size-5 text-muted-foreground"
              />
            </Link>
          </li>
        )}
      </ul>
      <div className="-mt-3 flex flex-col">
        <TelegramMenuItem variant="sheet" />
        <LogoutButton variant="sheet" />
      </div>
    </div>
  );
}
