// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Switch } from "./switch";

test("a switch is a switch named by its label, and Space turns it on", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<Switch label="Show boosts" onChange={(on) => changes.push(on)} />);
  const toggle = screen.getByRole("switch", { name: "Show boosts", checked: false });

  await user.tab();
  expect(document.activeElement).toBe(toggle);
  await user.keyboard(" ");

  expect(changes).toEqual([true]);
  expect(screen.getByRole("switch", { name: "Show boosts", checked: true })).toBe(toggle);
});
