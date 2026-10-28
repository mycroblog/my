// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Stepper } from "./Stepper";

describe("Stepper", () => {
  it("says the position in words and draws one segment per step, those reached filled", () => {
    const { container } = render(<Stepper current={2} total={3} label="Step 2 of 3" />);
    expect(screen.getByText("Step 2 of 3")).toBeVisible();
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- the segments are decorative and have no role.
    const segments = container.querySelectorAll("[aria-hidden='true'] > span");
    expect(segments).toHaveLength(3);
    expect(Array.from(segments).map((segment) => segment.hasAttribute("data-reached"))).toEqual([true, true, false]);
  });
});
