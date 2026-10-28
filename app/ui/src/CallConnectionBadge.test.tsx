// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CallConnectionBadge } from "./CallConnectionBadge";

describe("CallConnectionBadge", () => {
  it("shows the state's name in the tone of the state", () => {
    const { rerender } = render(<CallConnectionBadge state="connected" label="Connected" />);
    const badge = screen.getByText("Connected");
    expect(badge).toHaveClass("bg-success-bg");
    rerender(<CallConnectionBadge state="reconnecting" label="Reconnecting…" />);
    expect(screen.getByText("Reconnecting…")).toHaveClass("bg-warning-bg");
    expect(screen.getByText("Reconnecting…")).toHaveAttribute("data-connection", "reconnecting");
  });
});
