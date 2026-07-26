import { describe, expect, it } from "vitest";
import { settingsPagesForRole } from "./settings";

describe("settings navigation", () => {
  it("gives administrators focused configuration pages", () => {
    expect(settingsPagesForRole("administrator").map((page) => page.id)).toEqual([
      "access",
      "sites",
      "credentials",
      "agents",
      "notifications",
    ]);
  });

  it("limits viewers to their access page", () => {
    expect(settingsPagesForRole("viewer").map((page) => page.id)).toEqual(["access"]);
  });
});
