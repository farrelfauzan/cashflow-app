import { describe, expect, it } from "vitest";

import type { Member } from "@/lib/api/generated/model";

import { memberStatusView } from "./member-status";

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
