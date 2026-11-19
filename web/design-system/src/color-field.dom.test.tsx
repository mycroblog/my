// SPDX-License-Identifier: AGPL-3.0-only

// The field of a color token (#3258, ADR-0059), as assistive technology gets it: a group named by the token, a group
// a scheme, each scheme's color a field described by its pairs' verdicts and invalid where a pair is under its
// minimum, what is no color said once the reader leaves the field. How it stands at a width is its stories' (Color
// field).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { ColorField, type SchemeColors } from "./color-field";

const surface = { light: "#ffffff", dark: "#221c1b" };

/** Accent text over the surface, starting from `start`, and what the field told its owner. */
function field(start: SchemeColors) {
  const told: SchemeColors[] = [];
  function Field() {
    const [value, setValue] = useState(start);
    return (
      <ColorField
        label="Accent text"
        about="Links and the focus ring."
        value={value}
        onChange={(next) => {
          told.push(next);
          setValue(next);
        }}
        pairs={[{ label: "On surface", other: surface, role: "text", minimum: 4.5 }]}
      />
    );
  }
  render(<Field />);
  return told;
}

test("each scheme's color is a field described by its pair's ratio, and a pair under its minimum makes it invalid", () => {
  field({ light: "#ff6b6b", dark: "#ff6b6b" });
  const token = screen.getByRole("group", { name: "Accent text" });
  const light = within(within(token).getByRole("group", { name: "Light" })).getByRole("textbox", { name: "Light" });
  const dark = within(within(token).getByRole("group", { name: "Dark" })).getByRole("textbox", { name: "Dark" });

  expect(light.getAttribute("aria-invalid")).toBe("true");
  expect(dark.getAttribute("aria-invalid")).toBeNull();
  // What describes it, without what is shown alone (the pair's sample).
  const said = (input: HTMLElement) =>
    (input.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .map((id) => {
        const node = document.getElementById(id)?.cloneNode(true) as HTMLElement | undefined;
        node?.querySelectorAll('[aria-hidden="true"]').forEach((hidden) => {
          hidden.remove();
        });
        return node?.textContent;
      })
      .join(" ");
  // Cut, never rounded up: 2.77 stays under 2.78.
  expect(said(light)).toBe("On surface2.77:1, needs 4.5:1");
  expect(said(dark)).toBe("On surface6.05:1, meets AA");
});

test("a whole color typed tells the owner; what is no color is said once the reader leaves the field", async () => {
  const user = userEvent.setup();
  const told = field({ light: "#a64646", dark: "#ff6b6b" });
  const light = screen.getByRole("textbox", { name: "Light" });

  await user.clear(light);
  await user.type(light, "#ff6b6");
  expect(told).toEqual([]);
  expect(screen.queryByText(/characters from 0 to 9 and a to f/)).toBeNull();

  await user.tab();
  expect(light.getAttribute("aria-invalid")).toBe("true");
  expect(
    screen.getByText("Enter a color as # and 6 characters from 0 to 9 and a to f, such as #ff6b6b."),
  ).not.toBeNull();

  await user.type(light, "B");
  expect(told).toEqual([{ light: "#ff6b6b", dark: "#ff6b6b" }]);
  expect(screen.queryByText(/characters from 0 to 9 and a to f/)).toBeNull();
});
