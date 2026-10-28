import { House } from "lucide-react";

// A signed-in number with no active membership: never invited, or disabled everywhere.
export function NoOrganization() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-4 pb-20 text-center md:gap-5 md:pb-0">
      <span
        aria-hidden="true"
        className="flex size-16 items-center justify-center rounded-[18px] bg-secondary md:size-[72px] md:rounded-[20px]"
      >
        <House className="size-8 text-muted-foreground md:size-9" />
      </span>
      <h1 className="text-2xl font-bold md:text-[32px]">
        Belum ada usaha untuk akun ini
      </h1>
      <p className="max-w-[540px] text-base leading-relaxed text-muted-foreground md:text-[19px]">
        Nomor ini belum terdaftar di usaha mana pun, atau aksesnya sudah
        dihentikan. Minta admin usaha Anda menambahkan nomor ini, lalu masuk
        lagi.
      </p>
    </div>
  );
}
