// SPDX-License-Identifier: AGPL-3.0-only
// The tabs pattern's wiring (WAI-ARIA APG): the selected tab's aria-controls names the shown panel, which names the tab back.
import { screen } from "@testing-library/react";
import { expect } from "vitest";

/** Expects the tab called `name` to be selected and to control the tab panel it labels. */
export function expectSelectedTabControlsPanel(name: string | RegExp): void {
  const tab = screen.getByRole("tab", { name, selected: true });
  const panel = screen.getByRole("tabpanel", { name });
  expect(panel.id).not.toBe("");
  expect(tab).toHaveAttribute("aria-controls", panel.id);
  expect(panel).toHaveAttribute("aria-labelledby", tab.id);
}
