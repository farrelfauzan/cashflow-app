import { MoreMenu } from "@/components/client/shell/more-menu";
import { PhoneTabLink } from "@/components/client/shell/phone-tab-link";
import { PhoneVisitsTabLink } from "@/components/client/shell/phone-visits-tab-link";

import { OrgMoreMenu } from "./org-more-menu";
import type { OrgNavItem } from "./type";
import {
  isExactSection,
  MORE_MENU_LABEL,
  moreMenuSections,
  ORG_NAV_ICONS,
  phoneTabs,
} from "./utils";

type PhoneTabBarProps = {
  orgId: string;
  // The sections the member may open (visibleOrgMenu), as the side menu has them.
  items: readonly OrgNavItem[];
  userName: string;
  roleLine: string;
  canSwitch: boolean;
};

// The phone's navigation (boards HP-Tab, approved 2026-11-08, and HP-Tab-Karyawan): a bar fixed
// to the bottom with up to four sections the member may open (phoneTabs: field work first, then
// Beranda, Penyewa, Penagihan and Properti), under their short names, and "Lainnya" for the
// rest. Above the home indicator; hidden from the iPad width up, where the side menu stays. On iOS it floats as a rounded glass panel 12px from the sides (board
// iOS-HP-Tab, T-097e): solid with Increase Contrast; Android keeps the bar along the edge.
export function PhoneTabBar({
  orgId,
  items,
  userName,
  roleLine,
  canSwitch,
}: PhoneTabBarProps) {
  const more = moreMenuSections(items);
  return (
    <nav
      aria-label="Menu utama"
      data-glass="chrome"
      className="fixed inset-x-0 bottom-0 z-40 border-t bg-card px-1 pt-1.5 pb-[calc(env(safe-area-inset-bottom)+6px)] shadow-[0_-2px_8px_rgba(30,42,43,0.05)] [view-transition-name:phone-tab-bar] md:hidden ios:inset-x-3 ios:bottom-[max(24px,env(safe-area-inset-bottom))] ios:rounded-[30px] ios:border ios:p-1 ios:shadow-none"
    >
      <ul className="flex">
        {phoneTabs(items).map((item) => {
          const Icon = ORG_NAV_ICONS[item.key];
          // Kunjungan carries the count of visits still open; the other tabs carry none.
          const TabLink =
            item.key === "visits" ? (
              <PhoneVisitsTabLink
                orgId={orgId}
                href={item.href}
                label={item.tabLabel}
                icon={<Icon aria-hidden="true" />}
              />
            ) : (
              <PhoneTabLink
                href={item.href}
                exact={isExactSection(item.key)}
                label={item.tabLabel}
                icon={<Icon aria-hidden="true" />}
              />
            );
          return (
            <li key={item.key} className="flex min-w-0 flex-1">
              {TabLink}
            </li>
          );
        })}
        <li className="flex min-w-0 flex-1">
          <MoreMenu
            label={MORE_MENU_LABEL}
            sectionHrefs={more.map((item) => item.href)}
          >
            <OrgMoreMenu
              userName={userName}
              roleLine={roleLine}
              sections={more}
              canSwitch={canSwitch}
            />
          </MoreMenu>
        </li>
      </ul>
    </nav>
  );
}
