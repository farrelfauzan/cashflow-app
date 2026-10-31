"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { Controller } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import { SheetClose } from "@/components/ui/sheet";
import type { Member, Property } from "@/lib/api/generated/model";
import {
  getListMembersQueryKey,
  useSetMemberRoles,
} from "@/lib/api/generated/members/members";
import {
  ACCESS_FORM_API_FIELDS,
  accessFormError,
  accessFormSchema,
  roleFieldErrors,
  roleFormFromRoles,
} from "./utils";

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
  const { form, showApiError, alert } = useApiForm({
    schema: accessFormSchema,
    defaultValues: { roles: roleFormFromRoles(member.roles) },
    apiFields: ACCESS_FORM_API_FIELDS,
    apiError: accessFormError,
  });
  const save = useSetMemberRoles({
    mutation: {
      onSuccess: () =>
        queryClient.invalidateQueries({
          queryKey: getListMembersQueryKey(orgId),
        }),
    },
  });
  const saving = save.isPending || save.isSuccess;

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    save.mutate(
      { orgId, memberId: member.id, data: input },
      { onSuccess: onSaved, onError: showApiError },
    );
  });

  return (
    <>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-5">
        <form
          id={formId}
          noValidate
          onSubmit={submit}
          className="flex flex-col gap-4"
        >
          {alert !== undefined && <FormAlert message={alert} />}
          <Controller
            control={form.control}
            name="roles"
            render={({ field: roles }) => (
              <RoleFields
                mode="edit"
                value={roles.value}
                onChange={(next) => {
                  roles.onChange(next);
                  form.clearErrors("roles");
                }}
                properties={properties}
                errors={roleFieldErrors(form.formState.errors.roles)}
                disabled={saving}
              />
            )}
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
