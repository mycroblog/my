// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { type PostVisibility, WarningIcon } from "./Icons";
import { StatusBadge, VisibilityMark } from "./StatusBadge";

describe("StatusBadge", () => {
  it("shows the state's icon and its name in the tone's colors", () => {
    render(<StatusBadge icon={<WarningIcon filled />} label="Suspended" tone="danger" />);
    const badge = screen.getByText("Suspended");
    expect(badge).toHaveClass("bg-danger-bg", "text-danger");
    // eslint-disable-next-line testing-library/no-node-access -- the decorative icon has no role.
    expect(badge.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  });
});

describe("VisibilityMark", () => {
  const levels: readonly PostVisibility[] = ["public", "local", "unlisted", "private", "direct"];

  it("draws a different icon for every level, so that the shape tells them apart", () => {
    render(
      <>
        {levels.map((level) => (
          <VisibilityMark key={level} visibility={level} label={level} />
        ))}
      </>,
    );
    // eslint-disable-next-line testing-library/no-node-access -- the decorative icons have no role.
    const shapes = levels.map((level) => screen.getByText(level).querySelector("svg")?.innerHTML);
    expect(new Set(shapes).size).toBe(levels.length);
  });
});
