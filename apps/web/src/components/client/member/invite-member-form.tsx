"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState, type FormEvent } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
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
  inviteFormError,
  inviteRoleForm,
  roleFormErrors,
  roleGrantsFromForm,
  EMPTY_NAME_MESSAGE,
  EMPTY_PHONE_MESSAGE,
  INPUT_CLASS,
  NAME_MAX_LENGTH,
} from "./utils";
import type { RoleFormErrors, TextFieldErrors } from "./type";

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
  const nameRef = useRef<HTMLInputElement>(null);
  const phoneRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [roles, setRoles] = useState(inviteRoleForm);
  const [fieldErrors, setFieldErrors] = useState<TextFieldErrors>({});
  const [roleErrors, setRoleErrors] = useState<RoleFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);
  const invite = useInviteMember({
    mutation: {
      onSuccess: () =>
        queryClient.invalidateQueries({
          queryKey: getListMembersQueryKey(orgId),
        }),
    },
  });
  const sending = invite.isPending || invite.isSuccess;

  function focusField(field: keyof TextFieldErrors) {
    (field === "name" ? nameRef : phoneRef).current?.focus();
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (sending) return;

    setAlert(null);
    const typedName = name.trim();
    const typedPhone = phone.trim();
    const found: TextFieldErrors = {};
    if (typedName === "") found.name = EMPTY_NAME_MESSAGE;
    if (typedPhone === "") found.phone = EMPTY_PHONE_MESSAGE;
    const foundRoles = roleFormErrors(roles);
    setFieldErrors(found);
    setRoleErrors(foundRoles);
    if (found.name !== undefined) return focusField("name");
    if (found.phone !== undefined) return focusField("phone");
    if (Object.keys(foundRoles).length > 0) return;

    invite.mutate(
      {
        orgId,
        data: {
          name: typedName,
          phone: typedPhone,
          roles: roleGrantsFromForm(roles),
        },
      },
      {
        onSuccess: onInvited,
        onError: (error) => {
          const { field, message } = inviteFormError(error);
          if (field === "name" || field === "phone") {
            setFieldErrors({ [field]: message });
            focusField(field);
          } else if (field === "roles") {
            setRoleErrors({ roles: message });
          } else {
            setAlert(message);
          }
        },
      },
    );
  }

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={handleSubmit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== null && <FormAlert message={alert} />}

        <div className="flex flex-col gap-2">
          <Label
            htmlFor={`${id}-name`}
            className="text-base leading-normal font-semibold"
          >
            Nama
          </Label>
          <Input
            ref={nameRef}
            id={`${id}-name`}
            name="name"
            type="text"
            autoComplete="off"
            maxLength={NAME_MAX_LENGTH}
            placeholder="Contoh: Mas Rizal Fauzi"
            value={name}
            onChange={(event) => {
              setName(event.target.value);
              setFieldErrors(({ phone: phoneError }) => ({
                phone: phoneError,
              }));
            }}
            disabled={sending}
            aria-invalid={fieldErrors.name !== undefined}
            aria-describedby={
              fieldErrors.name !== undefined ? `${id}-name-error` : undefined
            }
            className={INPUT_CLASS}
          />
          {fieldErrors.name !== undefined && (
            <FieldError id={`${id}-name-error`} message={fieldErrors.name} />
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
            ref={phoneRef}
            id={`${id}-phone`}
            name="phone"
            type="tel"
            inputMode="tel"
            autoComplete="off"
            placeholder="0812-3456-7890"
            value={phone}
            onChange={(event) => {
              setPhone(event.target.value);
              setFieldErrors(({ name: nameError }) => ({ name: nameError }));
            }}
            disabled={sending}
            aria-invalid={fieldErrors.phone !== undefined}
            aria-describedby={
              fieldErrors.phone !== undefined
                ? `${id}-phone-error ${id}-phone-hint`
                : `${id}-phone-hint`
            }
            className={INPUT_CLASS}
          />
          {fieldErrors.phone !== undefined && (
            <FieldError id={`${id}-phone-error`} message={fieldErrors.phone} />
          )}
          <p
            id={`${id}-phone-hint`}
            className="text-[15px] leading-[1.45] text-muted-foreground"
          >
            Kami kirim undangan lewat WhatsApp ke nomor ini. Untuk masuk, mereka
            memakai nomor yang sama.
          </p>
        </div>

        <RoleFields
          mode="invite"
          value={roles}
          onChange={(next) => {
            setRoles(next);
            setRoleErrors({});
          }}
          properties={properties}
          errors={roleErrors}
          disabled={sending}
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
