import { describe, expect, it } from "vitest";

import type { Capability, Member } from "@/lib/api/generated/model";

import {
  canManageMember,
  canResendInvitation,
  memberPermissions,
  type MemberPermissions,
} from "./member-controls";

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
    expect(memberPermissions(capabilities)).toEqual(want);
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
