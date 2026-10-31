"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { Controller } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import type { Member, Property } from "@/lib/api/generated/model";
import {
  getListMembersQueryKey,
  useInviteMember,
} from "@/lib/api/generated/members/members";
import {
  INPUT_CLASS,
  INVITE_FORM_API_FIELDS,
  NAME_MAX_LENGTH,
  inviteFormError,
  inviteFormSchema,
  inviteRoleForm,
  roleFieldErrors,
} from "./utils";

import { RoleFields } from "./role-fields";

type InviteMemberFormProps = {
  orgId: string;
  properties: readonly Property[];
  // inviteMember answered 201; the invitation may or may not have gone out.
  onInvited: (member: Member) => void;
};

// "Tambah anggota": name, WhatsApp number and roles. "Tambah dan kirim undangan" adds the member
// and sends the WhatsApp invitation in one request.
export function InviteMemberForm({
  orgId,
  properties,
  onInvited,
}: InviteMemberFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const { form, field, showApiError, alert } = useApiForm({
    schema: inviteFormSchema,
    defaultValues: { name: "", phone: "", roles: inviteRoleForm() },
    apiFields: INVITE_FORM_API_FIELDS,
    apiError: inviteFormError,
  });
  const { errors } = form.formState;
  const nameError = errors.name?.message;
  const phoneError = errors.phone?.message;
  const invite = useInviteMember({
    mutation: {
      onSuccess: () =>
        queryClient.invalidateQueries({
          queryKey: getListMembersQueryKey(orgId),
        }),
    },
  });
  const sending = invite.isPending || invite.isSuccess;

  const submit = form.handleSubmit((input) => {
    if (sending) return;
    invite.mutate(
      { orgId, data: input },
      { onSuccess: onInvited, onError: showApiError },
    );
  });

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}

        <div className="flex flex-col gap-2">
          <Label
            htmlFor={`${id}-name`}
            className="text-base leading-normal font-semibold"
          >
            Nama
          </Label>
          <Input
            id={`${id}-name`}
            type="text"
            autoComplete="off"
            maxLength={NAME_MAX_LENGTH}
            placeholder="Contoh: Mas Rizal Fauzi"
            {...field("name")}
            disabled={sending}
            aria-invalid={nameError !== undefined}
            aria-describedby={
              nameError !== undefined ? `${id}-name-error` : undefined
            }
            className={INPUT_CLASS}
          />
          {nameError !== undefined && (
            <FieldError id={`${id}-name-error`} message={nameError} />
          )}
        </div>

        <div className="flex flex-col gap-2">
          <Label
            htmlFor={`${id}-phone`}
            className="text-base leading-normal font-semibold"
          >
            Nomor WhatsApp
          </Label>
          <Input
            id={`${id}-phone`}
            type="tel"
            inputMode="tel"
            autoComplete="off"
            placeholder="0812-3456-7890"
            {...field("phone")}
            disabled={sending}
            aria-invalid={phoneError !== undefined}
            aria-describedby={
              phoneError !== undefined
                ? `${id}-phone-error ${id}-phone-hint`
                : `${id}-phone-hint`
            }
            className={INPUT_CLASS}
          />
          {phoneError !== undefined && (
            <FieldError id={`${id}-phone-error`} message={phoneError} />
          )}
          <p
            id={`${id}-phone-hint`}
            className="text-[15px] leading-[1.45] text-muted-foreground"
          >
            Kami kirim undangan lewat WhatsApp ke nomor ini. Untuk masuk, mereka
            memakai nomor yang sama.
          </p>
        </div>

        <Controller
          control={form.control}
          name="roles"
          render={({ field: roles }) => (
            <RoleFields
              mode="invite"
              value={roles.value}
              onChange={(next) => {
                roles.onChange(next);
                form.clearErrors("roles");
              }}
              properties={properties}
              errors={roleFieldErrors(errors.roles)}
              disabled={sending}
            />
          )}
        />
      </form>
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
          disabled={sending}
          className="h-13 flex-2 rounded-xl px-3 text-[17px] font-semibold whitespace-normal"
        >
          {sending ? "Mengirim undangan…" : "Tambah dan kirim undangan"}
        </Button>
      </div>
    </>
  );
}
