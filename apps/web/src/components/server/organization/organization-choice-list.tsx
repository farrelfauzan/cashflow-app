import type { Organization } from "@/lib/api/generated/model";

import { OrganizationChoice } from "./organization-choice";

type OrganizationChoiceListProps = {
  organizations: readonly Organization[];
};

export function OrganizationChoiceList({
  organizations,
}: OrganizationChoiceListProps) {
  return (
    <div className="flex flex-1 flex-col gap-5 md:items-center md:justify-center md:gap-7">
      <div className="flex w-full flex-col gap-1.5 md:max-w-[560px] md:gap-2">
        <h1 className="text-[28px] font-bold tracking-tight md:text-4xl">
          Pilih usaha
        </h1>
        <p className="text-base text-muted-foreground md:text-[19px] md:leading-normal">
          Anda terdaftar di {organizations.length} usaha. Pilih yang ingin
          dibuka.
          <span className="hidden md:inline">
            {" "}
            Anda bisa pindah usaha kapan saja dari menu.
          </span>
        </p>
      </div>
      <ul
        aria-label="Daftar usaha"
        className="flex w-full flex-col gap-2.5 md:max-w-[560px] md:gap-3"
      >
        {organizations.map((organization) => (
          <li key={organization.id}>
            <OrganizationChoice organization={organization} />
          </li>
        ))}
      </ul>
    </div>
  );
}
