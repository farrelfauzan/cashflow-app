import type { MemberPanelState } from "@/lib/member/panel-state";

export type TextFieldErrors = Partial<Record<"name" | "phone", string>>;

export type OpenPanel = Exclude<MemberPanelState, { kind: "closed" }>;

export type Outcome = { message: string; failed: boolean };
