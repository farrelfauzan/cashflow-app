import { expect, type Locator } from "@playwright/test";

// A button's look by meaning (T-093, components/ui/button.tsx), read from its computed colors,
// so the test sees what a person sees rather than a class name. Values are the palette in
// src/app/globals.css.
const INK = "rgb(30, 42, 43)";
const CREAM = "rgb(245, 243, 238)";
const ACCENT = "rgb(29, 107, 99)";
const WARNING = "rgb(180, 83, 15)";
const ERROR = "rgb(163, 40, 28)";

export type ButtonTone =
  | "neutral"
  | "create"
  | "caution-outline"
  | "caution"
  | "destructive-outline"
  | "destructive";

const LOOKS: Record<
  ButtonTone,
  { color: string; background?: string; border?: string }
> = {
  neutral: { color: INK },
  create: { color: CREAM, background: ACCENT },
  "caution-outline": { color: WARNING, border: WARNING },
  caution: { color: CREAM, background: WARNING },
  "destructive-outline": { color: ERROR, border: ERROR },
  destructive: { color: CREAM, background: ERROR },
};

// Checked while the pointer is elsewhere: outlines fill solid on hover.
export async function expectButtonTone(
  button: Locator,
  tone: ButtonTone,
): Promise<void> {
  const look = LOOKS[tone];
  await expect(button).toHaveCSS("color", look.color);
  if (look.background !== undefined) {
    await expect(button).toHaveCSS("background-color", look.background);
  }
  if (look.border !== undefined) {
    await expect(button).toHaveCSS("border-top-color", look.border);
  }
}
