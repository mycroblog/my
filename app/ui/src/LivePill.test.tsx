// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LivePill } from "./LivePill";

describe("LivePill", () => {
  it("shows its label as text, read as it is shown", () => {
    render(<LivePill label="Live" />);
    expect(screen.getByText("Live")).toHaveClass("bg-accent-fill", "text-accent-fill-fg");
  });

  it("turns into a dashed outline while the user's call is interrupted", () => {
    render(<LivePill label="Live" interrupted />);
    expect(screen.getByText("Live")).toHaveClass("border-dashed", "text-accent");
    expect(screen.getByText("Live")).not.toHaveClass("bg-accent-fill");
  });
});
