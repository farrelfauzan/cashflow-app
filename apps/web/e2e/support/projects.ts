import type { TestInfo } from "@playwright/test";

// The size projects of playwright.config.ts (CLAUDE.md "UX rules" targets). They run in
// parallel against one database, so every record a test makes carries its project's own digit
// or word, and no two projects ever touch the same name, number or seeded person.

export type ProjectName = "ipad" | "phone" | "desktop";

type Project = {
  // In numbers a test makes (tenant and invite numbers, chat ids, prices).
  digit: "1" | "2" | "3";
  // In names a test makes ("Wisma Tetap … Satu").
  word: "Satu" | "Dua" | "Tiga";
  // Minimum body text, CLAUDE.md "UX rules": desktop like iPad.
  minBodyFontPx: number;
};

const PROJECTS: Record<ProjectName, Project> = {
  ipad: { digit: "1", word: "Satu", minBodyFontPx: 16 },
  phone: { digit: "2", word: "Dua", minBodyFontPx: 14 },
  desktop: { digit: "3", word: "Tiga", minBodyFontPx: 16 },
};

function isProjectName(name: string): name is ProjectName {
  return Object.hasOwn(PROJECTS, name);
}

// The test's size project; throws for any other project (setup, or one added without a row
// here), so a new project never silently shares another one's data.
export function projectName(testInfo: TestInfo): ProjectName {
  const name = testInfo.project.name;
  if (!isProjectName(name)) {
    throw new Error(`Unknown size project "${name}" (e2e/support/projects.ts)`);
  }
  return name;
}

// "1" for ipad, "2" for phone, "3" for desktop.
export function projectDigit(testInfo: TestInfo): Project["digit"] {
  return PROJECTS[projectName(testInfo)].digit;
}

// "Satu" for ipad, "Dua" for phone, "Tiga" for desktop.
export function projectWord(testInfo: TestInfo): Project["word"] {
  return PROJECTS[projectName(testInfo)].word;
}

// 16 on ipad and desktop, 14 on phone.
export function minBodyFontPx(testInfo: TestInfo): number {
  return PROJECTS[projectName(testInfo)].minBodyFontPx;
}
