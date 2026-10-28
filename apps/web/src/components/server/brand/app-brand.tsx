import { House } from "lucide-react";

import { APP_NAME } from "@/lib/app-name";

// The app's mark and name, as on the login boards.
export function AppBrand() {
  return (
    <div className="flex items-center gap-2.5 md:gap-3">
      <span
        aria-hidden="true"
        className="flex size-9 items-center justify-center rounded-[10px] bg-primary text-primary-foreground md:size-10 md:rounded-[11px]"
      >
        <House className="size-5 md:size-[22px]" />
      </span>
      <span className="text-base font-bold md:text-xl">{APP_NAME}</span>
    </div>
  );
}
