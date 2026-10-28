// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgeBadge } from "./AgeBadge";

describe("AgeBadge", () => {
  it("says the wait in words inside a time, its tone rising with the level", () => {
    render(<AgeBadge label="Waiting 4 days" dateTime="2026-10-01T09:00:00Z" title="1 October 2026" level="overdue" />);
    const time = screen.getByTitle("1 October 2026");
    expect(time.tagName).toBe("TIME");
    expect(time).toHaveAttribute("datetime", "2026-10-01T09:00:00Z");
    expect(screen.getByText("Waiting 4 days")).toHaveClass("bg-danger-bg");
  });

  it("is neutral while fresh", () => {
    render(<AgeBadge label="Waiting 5 min." dateTime="2026-10-01T09:00:00Z" title="1 October 2026" />);
    expect(screen.getByText("Waiting 5 min.")).toHaveClass("bg-surface-sunken");
  });
});
