// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SeverityMark } from "./SeverityMark";

describe("SeverityMark", () => {
  it("says the level in words and marks a choice that cannot be undone", () => {
    render(<SeverityMark level={10} max={10} label="Severity 10 of 10" irreversibleLabel="Cannot be undone" />);
    expect(screen.getByText("Severity 10 of 10")).toBeInTheDocument();
    expect(screen.getByText("Cannot be undone")).toHaveClass("bg-danger-bg");
  });

  it("has no badge for a choice that can be undone", () => {
    render(<SeverityMark level={1} max={10} label="Severity 1 of 10" />);
    expect(screen.queryByText("Cannot be undone")).not.toBeInTheDocument();
  });
});
