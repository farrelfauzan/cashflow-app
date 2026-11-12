import { MoreMenu } from "@/components/client/shell/more-menu";
import { PhoneTabLink } from "@/components/client/shell/phone-tab-link";

import { OrgMoreMenu } from "./org-more-menu";
import type { OrgNavItem } from "./type";
import {
  MORE_MENU_LABEL,
  moreMenuSections,
  ORG_NAV_ICONS,
  phoneTabs,
} from "./utils";

type PhoneTabBarProps = {
  // The sections the member may open (visibleOrgMenu), as the side menu has them.
  items: readonly OrgNavItem[];
  userName: string;
  roleLine: string;
  canSwitch: boolean;
};

// The phone's navigation (board HP-Tab, approved 2026-11-08): a bar fixed to the bottom with
// Beranda, Penyewa, Penagihan and Properti (each only for a member who may open it) and
// "Lainnya" for the rest. Above the home indicator; hidden from the iPad width up, where the
// side menu stays.
export function PhoneTabBar({
  items,
  userName,
  roleLine,
  canSwitch,
}: PhoneTabBarProps) {
  const more = moreMenuSections(items);
  return (
    <nav
      aria-label="Menu utama"
      className="fixed inset-x-0 bottom-0 z-40 border-t bg-card px-1 pt-1.5 pb-[calc(env(safe-area-inset-bottom)+6px)] shadow-[0_-2px_8px_rgba(30,42,43,0.05)] [view-transition-name:phone-tab-bar] md:hidden"
    >
      <ul className="flex">
        {phoneTabs(items).map((item) => {
          const Icon = ORG_NAV_ICONS[item.key];
          return (
            <li key={item.key} className="flex min-w-0 flex-1">
              <PhoneTabLink
                href={item.href}
                exact={item.key === "home"}
                label={item.label}
                icon={<Icon aria-hidden="true" />}
              />
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
