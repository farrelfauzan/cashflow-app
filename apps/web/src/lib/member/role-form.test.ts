import { describe, expect, it } from "vitest";

import type { RoleRef } from "@/lib/api/generated/model";

import {
  inviteRoleForm,
  NO_PROPERTY_MESSAGE,
  NO_ROLE_MESSAGE,
  roleFormErrors,
  roleFormFromRoles,
  roleGrantsFromForm,
  toggleProperty,
  type RoleFormState,
} from "./role-form";

const KOS_MELATI = "11111111-1111-4111-8111-111111111111";
const MAWAR = "22222222-2222-4222-8222-222222222222";

const supervisor: RoleRef = {
  key: "supervisor",
  name: "Supervisor",
  builtin: true,
};
const admin: RoleRef = { key: "admin", name: "Admin", builtin: true };
const karyawan: RoleRef = { key: "karyawan", name: "Karyawan", builtin: true };

describe("roleFormFromRoles and roleGrantsFromForm", () => {
  it.each<{ name: string; roles: RoleRef[]; want: RoleFormState }>([
    {
      name: "an Admin for the whole organization",
      roles: [admin],
      want: {
        supervisor: [],
        admin: { selected: true, scope: "organization", propertyIds: [] },
        karyawan: { selected: false, scope: "organization", propertyIds: [] },
      },
    },
    {
      name: "a Karyawan and an Admin limited to one property",
      roles: [karyawan, { ...admin, propertyId: MAWAR }],
      want: {
        supervisor: [],
        admin: { selected: true, scope: "properties", propertyIds: [MAWAR] },
        karyawan: { selected: true, scope: "organization", propertyIds: [] },
      },
    },
    {
      name: "a Karyawan for two properties",
      roles: [
        { ...karyawan, propertyId: KOS_MELATI },
        { ...karyawan, propertyId: MAWAR },
      ],
      want: {
        supervisor: [],
        admin: { selected: false, scope: "organization", propertyIds: [] },
        karyawan: {
          selected: true,
          scope: "properties",
          propertyIds: [KOS_MELATI, MAWAR],
        },
      },
    },
    {
      name: "a Supervisor",
      roles: [supervisor],
      want: {
        supervisor: [{ role: "supervisor" }],
        admin: { selected: false, scope: "organization", propertyIds: [] },
        karyawan: { selected: false, scope: "organization", propertyIds: [] },
      },
    },
    {
      name: "a custom role, which the form leaves out",
      roles: [{ key: "penagih", name: "Penagih", builtin: false }, karyawan],
      want: {
        supervisor: [],
        admin: { selected: false, scope: "organization", propertyIds: [] },
        karyawan: { selected: true, scope: "organization", propertyIds: [] },
      },
    },
  ])("reads $name", ({ roles, want }) => {
    expect(roleFormFromRoles(roles)).toEqual(want);
  });

  it.each<{ name: string; roles: RoleRef[] }>([
    { name: "an Admin for the whole organization", roles: [admin] },
    {
      name: "a Karyawan and an Admin limited to one property",
      roles: [karyawan, { ...admin, propertyId: MAWAR }],
    },
    {
      name: "a Karyawan for two properties",
      roles: [
        { ...karyawan, propertyId: KOS_MELATI },
        { ...karyawan, propertyId: MAWAR },
      ],
    },
    { name: "a Supervisor and an Admin", roles: [supervisor, admin] },
  ])("sends back the same grants, in any order, for $name", ({ roles }) => {
    const want = roles.map((role) =>
      role.propertyId === undefined
        ? { role: role.key }
        : { role: role.key, propertyId: role.propertyId },
    );
    const got = roleGrantsFromForm(roleFormFromRoles(roles));
    expect(got).toHaveLength(want.length);
    expect(got).toEqual(expect.arrayContaining(want));
  });

  it("gives one grant per chosen property", () => {
    const form: RoleFormState = {
      supervisor: [],
      admin: {
        selected: true,
        scope: "properties",
        propertyIds: [KOS_MELATI, MAWAR],
      },
      karyawan: { selected: false, scope: "organization", propertyIds: [] },
    };
    expect(roleGrantsFromForm(form)).toEqual([
      { role: "admin", propertyId: KOS_MELATI },
      { role: "admin", propertyId: MAWAR },
    ]);
  });

  it("ignores chosen properties when the role covers the whole organization", () => {
    const form: RoleFormState = {
      supervisor: [],
      admin: { selected: false, scope: "properties", propertyIds: [MAWAR] },
      karyawan: {
        selected: true,
        scope: "organization",
        propertyIds: [KOS_MELATI],
      },
    };
    expect(roleGrantsFromForm(form)).toEqual([{ role: "karyawan" }]);
  });

  it("starts an invite as Karyawan for the whole organization", () => {
    expect(roleGrantsFromForm(inviteRoleForm())).toEqual([
      { role: "karyawan" },
    ]);
  });
});

describe("roleFormErrors", () => {
  it.each<{ name: string; form: RoleFormState; want: object }>([
    { name: "the invite default", form: inviteRoleForm(), want: {} },
    {
      name: "no role at all",
      form: {
        ...inviteRoleForm(),
        karyawan: { selected: false, scope: "organization", propertyIds: [] },
      },
      want: { roles: NO_ROLE_MESSAGE },
    },
    {
      name: "only a Supervisor",
      form: {
        supervisor: [{ role: "supervisor" }],
        admin: { selected: false, scope: "organization", propertyIds: [] },
        karyawan: { selected: false, scope: "organization", propertyIds: [] },
      },
      want: {},
    },
    {
      name: "a property limit without a property",
      form: {
        supervisor: [],
        admin: { selected: true, scope: "properties", propertyIds: [] },
        karyawan: { selected: true, scope: "properties", propertyIds: [MAWAR] },
      },
      want: { admin: NO_PROPERTY_MESSAGE },
    },
    {
      name: "a property limit on a role that is not chosen",
      form: {
        supervisor: [],
        admin: { selected: false, scope: "properties", propertyIds: [] },
        karyawan: { selected: true, scope: "organization", propertyIds: [] },
      },
      want: {},
    },
  ])("finds $name", ({ form, want }) => {
    expect(roleFormErrors(form)).toEqual(want);
  });
});

describe("toggleProperty", () => {
  it.each([
    {
      name: "adds a property",
      ids: [MAWAR],
      id: KOS_MELATI,
      checked: true,
      want: [MAWAR, KOS_MELATI],
    },
    {
      name: "never adds one twice",
      ids: [MAWAR],
      id: MAWAR,
      checked: true,
      want: [MAWAR],
    },
    {
      name: "removes a property",
      ids: [MAWAR, KOS_MELATI],
      id: MAWAR,
      checked: false,
      want: [KOS_MELATI],
    },
  ])("$name", ({ ids, id, checked, want }) => {
    expect(toggleProperty(ids, id, checked)).toEqual(want);
  });
});
