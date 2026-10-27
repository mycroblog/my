// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LivePill } from "./LivePill";

describe("LivePill", () => {
  it("shows its label as text, read as it is shown", () => {
    render(<LivePill label="Live" />);
    expect(screen.getByText("Live")).toHaveClass("bg-accent-fill", "text-accent-fill-fg");
  });
});
