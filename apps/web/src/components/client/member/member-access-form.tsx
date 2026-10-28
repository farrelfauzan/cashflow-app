"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useState, type FormEvent } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";
import type { Member, Property } from "@/lib/api/generated/model";
import {
  getListMembersQueryKey,
  useSetMemberRoles,
} from "@/lib/api/generated/members/members";
import { accessFormError } from "@/lib/member/member-errors";
import {
  roleFormErrors,
  roleFormFromRoles,
  roleGrantsFromForm,
  type RoleFormErrors,
} from "@/lib/member/role-form";

import { MemberStatusSection } from "./member-status-section";
import { RoleFields } from "./role-fields";

type MemberAccessFormProps = {
  orgId: string;
  organizationName: string;
  member: Member;
  properties: readonly Property[];
  onSaved: (member: Member) => void;
};

// "Atur akses": the member's roles ("Simpan peran", the panel's primary action), then "Hentikan
// akses" to disable the member or enable them again.
export function MemberAccessForm({
  orgId,
  organizationName,
  member,
  properties,
  onSaved,
}: MemberAccessFormProps) {
  const formId = useId();
  const queryClient = useQueryClient();
  const [form, setForm] = useState(() => roleFormFromRoles(member.roles));
  const [errors, setErrors] = useState<RoleFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);
  const save = useSetMemberRoles({
    mutation: {
      onSuccess: () =>
        queryClient.invalidateQueries({
          queryKey: getListMembersQueryKey(orgId),
        }),
    },
  });
  const saving = save.isPending || save.isSuccess;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const found = roleFormErrors(form);
    setErrors(found);
    if (Object.keys(found).length > 0) return;

    save.mutate(
      {
        orgId,
        memberId: member.id,
        data: { roles: roleGrantsFromForm(form) },
      },
      {
        onSuccess: onSaved,
        onError: (error) => {
          const { field, message } = accessFormError(error);
          if (field === "roles") setErrors({ roles: message });
          else setAlert(message);
        },
      },
    );
  }

  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-5">
        <form
          id={formId}
          noValidate
          onSubmit={handleSubmit}
          className="flex flex-col gap-4"
        >
          {alert !== null && <FormAlert message={alert} />}
          <RoleFields
            mode="edit"
            value={form}
            onChange={(next) => {
              setForm(next);
              setErrors({});
            }}
            properties={properties}
            errors={errors}
            disabled={saving}
          />
          <p className="rounded-[10px] bg-secondary/60 px-3.5 py-3 text-[15px] leading-normal text-foreground/85">
            Peran yang dibatasi ke properti hanya berlaku untuk data properti
            itu. Anggota tim, pengaturan, dan gaji tetap butuh peran untuk
            seluruh usaha.
          </p>
        </form>
        <MemberStatusSection
          orgId={orgId}
          organizationName={organizationName}
          member={member}
        />
      </div>
      <div className="flex gap-2.5 border-t px-6 py-4">
        <SheetClose asChild>
          <Button
            type="button"
            variant="outline"
            className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold"
          >
            Batal
          </Button>
        </SheetClose>
        <Button
          type="submit"
          form={formId}
          disabled={saving}
          className="h-13 flex-2 rounded-xl text-[17px] font-semibold"
        >
          {saving ? "Menyimpan…" : "Simpan peran"}
        </Button>
      </div>
    </>
  );
}
