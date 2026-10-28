"use client";

import { Check, TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";
import type { Member } from "@/lib/api/generated/model";
import { formatPhone } from "@/lib/format/phone";
import { describeRoles } from "./utils";
import type { PropertyNames } from "./type";
import { cn } from "@/lib/utils";

type InviteResultProps = {
  // The member inviteMember returned.
  member: Member;
  propertyNames: PropertyNames;
};

// "Anggota ditambahkan": the member is in the organization either way; the invitation either
// went out over WhatsApp or did not, and then can be resent from the list.
export function InviteResult({ member, propertyNames }: InviteResultProps) {
  const sent = member.invitation?.state === "sent";
  const roles = describeRoles(member.roles, propertyNames);
  const phone = formatPhone(member.phone);

  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-7">
        <div
          role="status"
          className={cn(
            "flex items-start gap-3.5 rounded-[14px] p-4",
            sent ? "bg-accent" : "bg-warning/10",
          )}
        >
          <span
            aria-hidden="true"
            className={cn(
              "flex size-10 shrink-0 items-center justify-center rounded-full",
              sent
                ? "bg-primary text-primary-foreground"
                : "bg-warning text-warning-foreground",
            )}
          >
            {sent ? (
              <Check className="size-[22px]" strokeWidth={2.4} />
            ) : (
              <TriangleAlert className="size-5" strokeWidth={2.2} />
            )}
          </span>
          <div className="flex flex-col gap-1">
            <p
              className={cn(
                "text-lg font-bold",
                sent ? "text-primary" : "text-foreground",
              )}
            >
              {sent
                ? "Undangan terkirim lewat WhatsApp"
                : "Undangan belum terkirim"}
            </p>
            <p className="text-base leading-normal text-foreground">
              {sent
                ? `${member.name} sudah ditambahkan sebagai ${roles}. Pesan undangan dikirim ke ${phone}.`
                : `${member.name} sudah ditambahkan sebagai ${roles}, tetapi pesan undangan lewat WhatsApp ke ${phone} belum terkirim.`}
            </p>
          </div>
        </div>
        <p className="text-base leading-[1.55] text-foreground/85">
          {sent
            ? `${member.name} bisa langsung masuk dengan nomor itu. Jika pesannya tidak sampai, kirim ulang dari daftar anggota.`
            : `Kirim ulang undangannya nanti dari daftar anggota. ${member.name} tetap bisa masuk dengan nomor itu.`}
        </p>
      </div>
      <div className="flex border-t px-6 py-4">
        <SheetClose asChild>
          <Button
            type="button"
            className="h-13 flex-1 rounded-xl text-[17px] font-semibold"
          >
            Selesai
          </Button>
        </SheetClose>
      </div>
    </>
  );
}
