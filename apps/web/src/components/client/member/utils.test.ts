import { describe, expect, it } from "vitest";

import {
  ApiError,
  GENERIC_ERROR_MESSAGE,
  networkError,
} from "@/lib/api/errors";
import type { Capability, Member, RoleRef } from "@/lib/api/generated/model";
import { createAppAbility } from "@/lib/capabilities";

import type { MemberPermissions, RoleFormState } from "./type";
import {
  accessFormError,
  canManageMember,
  canResendInvitation,
  describeRoles,
  inviteFormError,
  inviteRoleForm,
  memberPermissions,
  memberStatusView,
  NO_PROPERTY_MESSAGE,
  NO_ROLE_MESSAGE,
  resendError,
  roleChips,
  roleFormErrors,
  roleFormFromRoles,
  roleGrantsFromForm,
  toggleProperty,
  UNDELIVERABLE_HINT,
  UNKNOWN_PROPERTY,
} from "./utils";

describe("member controls", () => {
  const OWN_ID = "55555555-5555-4555-8555-555555555555";

  const invited: Member = {
    id: "33333333-3333-4333-8333-333333333333",
    userId: "44444444-4444-4444-8444-444444444444",
    name: "Mas Rizal Fauzi",
    phone: "+6281200010007",
    status: "active",
    roles: [{ key: "karyawan", name: "Karyawan", builtin: true }],
    createdAt: "2026-10-27T03:00:00Z",
    invitation: { state: "sent", lastSentAt: "2026-10-27T03:00:05Z" },
    firstSignInAt: null,
    collectorLeaseCount: 0,
  };

  const ALL: MemberPermissions = { canInvite: true, canManage: true };
  const NONE: MemberPermissions = { canInvite: false, canManage: false };

  describe("memberPermissions", () => {
    it.each<{
      name: string;
      capabilities: Capability[];
      want: MemberPermissions;
    }>([
      {
        name: "an Admin",
        capabilities: [
          { action: "member:read", access: "allow" },
          { action: "member:invite", access: "allow" },
          { action: "member:manage", access: "allow" },
        ],
        want: ALL,
      },
      {
        name: "a Supervisor, who only reads",
        capabilities: [
          { action: "member:read", access: "allow" },
          { action: "member:invite", access: "deny" },
          { action: "member:manage", access: "deny" },
        ],
        want: NONE,
      },
      {
        name: "conditional access, which shows the controls",
        capabilities: [
          { action: "member:invite", access: "conditional" },
          { action: "member:manage", access: "conditional" },
        ],
        want: ALL,
      },
      { name: "actions the API does not list", capabilities: [], want: NONE },
    ])("reads $name", ({ capabilities, want }) => {
      expect(memberPermissions(createAppAbility(capabilities))).toEqual(want);
    });
  });

  describe("canManageMember", () => {
    it.each([
      { name: "another member", member: invited, permissions: ALL, want: true },
      {
        name: "the signed-in member's own row",
        member: { ...invited, id: OWN_ID },
        permissions: ALL,
        want: false,
      },
      {
        name: "a disabled member, to enable again",
        member: { ...invited, status: "disabled" as const },
        permissions: ALL,
        want: true,
      },
      {
        name: "without member:manage",
        member: invited,
        permissions: NONE,
        want: false,
      },
    ])("$name: $want", ({ member, permissions, want }) => {
      expect(canManageMember(member, OWN_ID, permissions)).toBe(want);
    });
  });

  describe("canResendInvitation", () => {
    it.each([
      {
        name: "an invited member",
        member: invited,
        permissions: ALL,
        want: true,
      },
      {
        name: "an invitation that did not go out",
        member: { ...invited, invitation: { state: "not_sent" as const } },
        permissions: ALL,
        want: true,
      },
      {
        name: "a member who has signed in",
        member: { ...invited, firstSignInAt: "2026-10-27T04:00:00Z" },
        permissions: ALL,
        want: false,
      },
      {
        name: "a founder, never invited",
        member: { ...invited, invitation: null },
        permissions: ALL,
        want: false,
      },
      {
        name: "a disabled member",
        member: { ...invited, status: "disabled" as const },
        permissions: ALL,
        want: false,
      },
      {
        name: "without member:invite",
        member: invited,
        permissions: { canInvite: false, canManage: true },
        want: false,
      },
    ])("$name: $want", ({ member, permissions, want }) => {
      expect(canResendInvitation(member, permissions)).toBe(want);
    });
  });
});

describe("member errors", () => {
  function apiError(
    status: number,
    code: string,
    message: string,
    field?: string,
  ): ApiError {
    return new ApiError({ status, code, message, field });
  }

  const FORBIDDEN = apiError(
    403,
    "forbidden",
    "Anda tidak punya akses untuk tindakan ini.",
  );

  describe("inviteFormError", () => {
    it.each([
      {
        name: "an invalid phone, under the phone",
        error: apiError(
          400,
          "invalid_phone",
          "Nomor WhatsApp tidak valid.",
          "phone",
        ),
        want: { field: "phone", message: "Nomor WhatsApp tidak valid." },
      },
      {
        name: "a missing name, under the name",
        error: apiError(400, "invalid_input", "Nama wajib diisi.", "name"),
        want: { field: "name", message: "Nama wajib diisi." },
      },
      {
        name: "an unknown property, under the roles",
        error: apiError(
          400,
          "invalid_input",
          "Properti tidak ditemukan.",
          "roles",
        ),
        want: { field: "roles", message: "Properti tidak ditemukan." },
      },
      {
        name: "a number that is already a member, in the alert",
        error: apiError(
          409,
          "already_member",
          "Nomor ini sudah terdaftar di usaha ini.",
          "phone",
        ),
        want: {
          field: null,
          message: "Nomor ini sudah terdaftar di usaha ini.",
        },
      },
      {
        name: "a 403, in the alert",
        error: FORBIDDEN,
        want: { field: null, message: FORBIDDEN.message },
      },
      {
        name: "no connection, in the alert",
        error: networkError(),
        want: { field: null, message: networkError().message },
      },
    ])("puts $name", ({ error, want }) => {
      expect(inviteFormError(error)).toEqual(want);
    });
  });

  describe("accessFormError", () => {
    it.each([
      {
        name: "the last Admin, in the alert",
        error: apiError(
          409,
          "last_admin",
          "Anggota ini admin terakhir yang aktif. Tambahkan admin lain dulu.",
        ),
        want: {
          field: null,
          message:
            "Anggota ini admin terakhir yang aktif. Tambahkan admin lain dulu.",
        },
      },
      {
        name: "invalid roles, under the roles",
        error: apiError(
          400,
          "invalid_input",
          "Pilih minimal satu peran.",
          "roles",
        ),
        want: { field: "roles", message: "Pilih minimal satu peran." },
      },
      {
        name: "invalid input on another field, in the alert",
        error: apiError(
          400,
          "invalid_input",
          "Status harus active atau disabled.",
          "status",
        ),
        want: { field: null, message: "Status harus active atau disabled." },
      },
      {
        name: "a 403, in the alert",
        error: FORBIDDEN,
        want: { field: null, message: FORBIDDEN.message },
      },
      {
        name: "a bug, with the generic message",
        error: new Error("boom"),
        want: { field: null, message: GENERIC_ERROR_MESSAGE },
      },
    ])("puts $name", ({ error, want }) => {
      expect(accessFormError(error)).toEqual(want);
    });
  });

  describe("resendError", () => {
    it.each([
      {
        name: "an undeliverable number, without a retry",
        error: apiError(
          422,
          "whatsapp_undeliverable",
          "Nomor ini tidak bisa menerima WhatsApp.",
          "phone",
        ),
        want: {
          message: `Nomor ini tidak bisa menerima WhatsApp. ${UNDELIVERABLE_HINT}`,
          canRetry: false,
        },
      },
      {
        name: "a rate limit",
        error: apiError(
          429,
          "rate_limited",
          "Terlalu sering. Coba lagi nanti.",
        ),
        want: { message: "Terlalu sering. Coba lagi nanti.", canRetry: true },
      },
      {
        name: "WhatsApp down",
        error: apiError(
          503,
          "whatsapp_unavailable",
          "WhatsApp gagal mengirim. Coba lagi.",
        ),
        want: {
          message: "WhatsApp gagal mengirim. Coba lagi.",
          canRetry: true,
        },
      },
      {
        name: "a disabled member",
        error: apiError(409, "member_disabled", "Anggota ini sedang nonaktif."),
        want: { message: "Anggota ini sedang nonaktif.", canRetry: true },
      },
    ])("shows $name", ({ error, want }) => {
      expect(resendError(error)).toEqual(want);
    });
  });
});

describe("member status", () => {
  const base: Member = {
    id: "33333333-3333-4333-8333-333333333333",
    userId: "44444444-4444-4444-8444-444444444444",
    name: "Mas Rizal Fauzi",
    phone: "+6281200010007",
    status: "active",
    roles: [{ key: "karyawan", name: "Karyawan", builtin: true }],
    createdAt: "2026-10-27T03:00:00Z",
    invitation: { state: "sent", lastSentAt: "2026-10-27T03:00:05Z" },
    firstSignInAt: null,
    collectorLeaseCount: 0,
  };

  describe("memberStatusView", () => {
    it.each<{ name: string; member: Member; want: object }>([
      {
        name: "a member who has opened the organization",
        member: { ...base, firstSignInAt: "2026-10-27T04:00:00Z" },
        want: { label: "Aktif", tone: "active", note: null },
      },
      {
        name: "an invited member who has not signed in yet",
        member: base,
        want: {
          label: "Belum pernah masuk",
          tone: "pending",
          note: { text: "Undangan terkirim 27 Okt 2026", tone: "muted" },
        },
      },
      {
        name: "an invitation sent late in the evening, dated in Jakarta",
        member: {
          ...base,
          invitation: { state: "sent", lastSentAt: "2026-10-27T18:30:00Z" },
        },
        want: {
          label: "Belum pernah masuk",
          tone: "pending",
          note: { text: "Undangan terkirim 28 Okt 2026", tone: "muted" },
        },
      },
      {
        name: "an invitation that did not go out",
        member: { ...base, invitation: { state: "not_sent" } },
        want: {
          label: "Belum pernah masuk",
          tone: "pending",
          note: { text: "Undangan belum terkirim", tone: "warning" },
        },
      },
      {
        name: "an invitation that went out once and failed the last time",
        member: {
          ...base,
          invitation: { state: "not_sent", lastSentAt: "2026-10-27T03:00:05Z" },
        },
        want: {
          label: "Belum pernah masuk",
          tone: "pending",
          note: { text: "Undangan belum terkirim", tone: "warning" },
        },
      },
      {
        name: "a founder who has not signed in yet",
        member: { ...base, invitation: null },
        want: { label: "Belum pernah masuk", tone: "pending", note: null },
      },
      {
        name: "a disabled member who never signed in",
        member: { ...base, status: "disabled" },
        want: { label: "Nonaktif", tone: "disabled", note: null },
      },
      {
        name: "a disabled member who had signed in",
        member: {
          ...base,
          status: "disabled",
          firstSignInAt: "2026-10-27T04:00:00Z",
        },
        want: { label: "Nonaktif", tone: "disabled", note: null },
      },
      {
        name: "a disabled member who still collects from tenants",
        member: { ...base, status: "disabled", collectorLeaseCount: 3 },
        want: {
          label: "Nonaktif",
          tone: "disabled",
          note: { text: "3 penyewa belum punya penagih", tone: "warning" },
        },
      },
      {
        name: "an active collector, with no flag",
        member: {
          ...base,
          firstSignInAt: "2026-10-27T04:00:00Z",
          collectorLeaseCount: 2,
        },
        want: { label: "Aktif", tone: "active", note: null },
      },
    ])("shows $name", ({ member, want }) => {
      expect(memberStatusView(member)).toEqual(want);
    });
  });
});

describe("role form", () => {
  const KOS_MELATI = "11111111-1111-4111-8111-111111111111";
  const MAWAR = "22222222-2222-4222-8222-222222222222";

  const supervisor: RoleRef = {
    key: "supervisor",
    name: "Supervisor",
    builtin: true,
  };
  const admin: RoleRef = { key: "admin", name: "Admin", builtin: true };
  const karyawan: RoleRef = {
    key: "karyawan",
    name: "Karyawan",
    builtin: true,
  };

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
          karyawan: {
            selected: true,
            scope: "properties",
            propertyIds: [MAWAR],
          },
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
});

describe("role labels", () => {
  const KOS_MELATI = "11111111-1111-4111-8111-111111111111";
  const MAWAR = "22222222-2222-4222-8222-222222222222";

  const admin: RoleRef = { key: "admin", name: "Admin", builtin: true };
  const karyawan: RoleRef = {
    key: "karyawan",
    name: "Karyawan",
    builtin: true,
  };
  const inMelati = { propertyId: KOS_MELATI, propertyName: "Kos Melati" };
  const inMawar = { propertyId: MAWAR, propertyName: "Kontrakan Mawar" };

  describe("roleChips", () => {
    it("names the property of a limited role from the API", () => {
      expect(roleChips([karyawan, { ...admin, ...inMawar }])).toEqual([
        { id: "karyawan:", key: "karyawan", label: "Karyawan", property: null },
        {
          id: `admin:${MAWAR}`,
          key: "admin",
          label: "Admin",
          property: "Kontrakan Mawar",
        },
      ]);
    });

    it("falls back when the API sends no property name", () => {
      expect(roleChips([{ ...admin, propertyId: MAWAR }])[0]?.property).toBe(
        UNKNOWN_PROPERTY,
      );
    });

    it("uses the API's role name, also for a custom role", () => {
      expect(
        roleChips([{ key: "penagih", name: "Penagih", builtin: false }])[0]
          ?.label,
      ).toBe("Penagih");
    });
  });

  describe("describeRoles", () => {
    it.each<{ name: string; roles: RoleRef[]; want: string }>([
      { name: "no role", roles: [], want: "" },
      { name: "one role", roles: [karyawan], want: "Karyawan" },
      {
        name: "a limited role",
        roles: [{ ...karyawan, ...inMawar }],
        want: "Karyawan di Kontrakan Mawar",
      },
      {
        name: "two roles",
        roles: [{ ...admin, ...inMelati }, karyawan],
        want: "Admin di Kos Melati dan Karyawan",
      },
      {
        name: "three roles",
        roles: [{ ...admin, ...inMelati }, { ...admin, ...inMawar }, karyawan],
        want: "Admin di Kos Melati, Admin di Kontrakan Mawar dan Karyawan",
      },
    ])("describes $name", ({ roles, want }) => {
      expect(describeRoles(roles)).toBe(want);
    });
  });
});
