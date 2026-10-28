type PendingSectionProps = {
  title: string;
  description?: string;
};

// A section of the menu whose screen is not built yet. Each is replaced by its screen (W5 to
// W10).
export function PendingSection({
  title,
  description = "Halaman ini sedang disiapkan.",
}: PendingSectionProps) {
  return (
    <div className="flex flex-col gap-1.5">
      <h1 className="text-[26px] font-bold tracking-tight md:text-[34px]">
        {title}
      </h1>
      <p className="text-[15px] text-muted-foreground md:text-[17px]">
        {description}
      </p>
    </div>
  );
}
