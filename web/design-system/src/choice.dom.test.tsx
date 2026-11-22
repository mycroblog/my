// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen } from "@testing-library/react";
import { ChoiceList } from "./choice";
import { PreviewItems } from "./preview";

const items = [
  { key: "a", label: "Sourdough" },
  { key: "b", label: "Rye" },
  { key: "c", label: "Baguette" },
];

const none = () => undefined;

/** The keys of the rows a story's preview shows pressed, by their options' labels. */
const pressedRows = () =>
  ["Sourdough", "Rye", "Baguette"].filter(
    (name) => screen.getByRole("radio", { name }).closest("label")?.hasAttribute("data-pressed") ?? false,
  );

test("a row a story's preview presses by its key shows pressed, and the others rest", () => {
  render(
    <PreviewItems states={{ c: "pressed" }}>
      <ChoiceList label="Bread" multiple={false} items={items} chosen={[1]} onChange={none} />
    </PreviewItems>,
  );

  expect(pressedRows()).toEqual(["Baguette"]);
});

test("a choice list shows no row pressed outside a preview", () => {
  render(<ChoiceList label="Bread" multiple={false} items={items} chosen={[1]} onChange={none} />);

  expect(pressedRows()).toEqual([]);
});

test("a disabled choice list shows no row pressed, even where a preview presses one", () => {
  render(
    <PreviewItems states={{ c: "pressed" }}>
      <ChoiceList label="Bread" multiple={false} items={items} chosen={[0]} onChange={none} disabled />
    </PreviewItems>,
  );

  expect(pressedRows()).toEqual([]);
});
