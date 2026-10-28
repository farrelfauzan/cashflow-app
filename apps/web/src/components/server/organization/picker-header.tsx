import { LogoutButton } from "@/components/client/auth/logout-button";
import { AppBrand } from "@/components/server/brand/app-brand";
import { formatPhone } from "@/lib/format/phone";

type PickerHeaderProps = {
  name: string;
  // E.164, as the API sends it.
  phone: string;
};

// Who is signed in, and the way out. The brand shows on iPad only, as on the boards.
export function PickerHeader({ name, phone }: PickerHeaderProps) {
  return (
    <header className="flex items-center justify-between gap-3">
      <div className="hidden md:block">
        <AppBrand />
      </div>
      <div className="flex min-w-0 flex-1 items-center justify-between gap-4 md:flex-none">
        <div className="flex min-w-0 flex-col md:items-end">
          <span className="truncate text-[15px] font-semibold md:text-base">
            {name}
          </span>
          <span className="text-sm text-muted-foreground md:text-[15px]">
            {formatPhone(phone)}
          </span>
        </div>
        <LogoutButton />
      </div>
    </header>
  );
}
