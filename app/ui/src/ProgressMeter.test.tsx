// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ProgressMeter } from "./ProgressMeter";

describe("ProgressMeter", () => {
  it("is a progress bar named by its label, with its value in words", () => {
    render(
      <ProgressMeter kind="progress" label="Deliveries sent" value={1234} maxValue={5000} valueText="1,234 / 5,000" />,
    );
    const bar = screen.getByRole("progressbar", { name: "Deliveries sent" });
    expect(bar).toHaveAttribute("aria-valuenow", "1234");
    expect(bar).toHaveAttribute("aria-valuemax", "5000");
    expect(bar).toHaveAttribute("aria-valuetext", "1,234 / 5,000");
  });

  it("is a meter for how full something is", () => {
    render(
      <ProgressMeter kind="meter" label="Storage used" value={4} maxValue={5} valueText="4 / 5 GB" tone="warning" />,
    );
    expect(screen.getByRole("meter", { name: "Storage used" })).toHaveAttribute("aria-valuetext", "4 / 5 GB");
  });
});
