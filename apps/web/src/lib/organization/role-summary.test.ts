import { describe, expect, it } from "vitest";

import { roleSummary } from "./role-summary";

describe("roleSummary", () => {
  it("joins role names in the API's order", () => {
    expect(
      roleSummary([
        { key: "karyawan", name: "Karyawan", builtin: true },
        { key: "admin", name: "Admin", builtin: true, propertyId: "p-1" },
      ]),
    ).toBe("Karyawan, Admin");
  });

  it("shows a role held for several properties once", () => {
    expect(
      roleSummary([
        { key: "admin", name: "Admin", builtin: true, propertyId: "p-1" },
        { key: "admin", name: "Admin", builtin: true, propertyId: "p-2" },
      ]),
    ).toBe("Admin");
  });

  it("is empty without roles", () => {
    expect(roleSummary([])).toBe("");
  });
});
