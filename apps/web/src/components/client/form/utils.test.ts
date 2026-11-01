import { describe, expect, it } from "vitest";

import { searchSelectShownLabel } from "./utils";

describe("searchSelectShownLabel", () => {
  const options = [
    { value: "p1", label: "Kos Melati" },
    { value: "p2", label: "Kos Anggrek" },
  ];

  it.each([
    {
      name: "nothing chosen",
      value: "",
      remembered: "Kos Mawar",
      want: undefined,
    },
    {
      name: "a choice in the list",
      value: "p2",
      remembered: "Kos Mawar",
      want: "Kos Anggrek",
    },
    {
      name: "a choice the search moved out of the list",
      value: "p9",
      remembered: "Kos Mawar",
      want: "Kos Mawar",
    },
    {
      name: "a choice nobody remembers",
      value: "p9",
      remembered: undefined,
      want: undefined,
    },
  ])("shows $name", ({ value, remembered, want }) => {
    expect(searchSelectShownLabel(value, options, remembered)).toBe(want);
  });
});
