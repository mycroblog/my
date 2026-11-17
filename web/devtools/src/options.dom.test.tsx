// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { Options } from "./options";

test("Options draws the first variant alone, and names itself, the variant drawn and them all for the overlay", () => {
  render(<Options id="swatch" variants={{ square: <b>square</b>, round: <i>round</i> }} />);
  const drawn = screen.getByText("square");

  expect(screen.queryAllByText("round")).toHaveLength(0);
  // What the overlay reads to offer the variants: the holder's data attributes.
  const holder = drawn.parentElement;
  expect(holder === null ? undefined : Object.fromEntries(Object.entries(holder.dataset))).toEqual({
    myDevtoolsOptions: "swatch",
    variant: "square",
    variants: JSON.stringify(["square", "round"]),
  });
});
