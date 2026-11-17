// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "react-aria-components";
import { Chip, ChipRow } from "./chip";

test("a chip is a toggle named by its label that shows its glyph and count, and a click, Enter and Space each press it once", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(<Chip label="Party popper, 5" glyph="🎉" count={5} pressed={false} locale="en" onPress={() => presses++} />);
  const chip = screen.getByRole("button", { name: "Party popper, 5", pressed: false });
  expect(chip.textContent).toBe("🎉5");

  await user.tab();
  expect(document.activeElement).toBe(chip);
  await user.keyboard("{Enter}");
  expect(presses).toBe(1);
  await user.keyboard(" ");
  expect(presses).toBe(2);
  await user.click(chip);
  expect(presses).toBe(3);
});

test("a chip that is the reader's own says it is on", () => {
  render(<Chip label="Party popper, 6" glyph="🎉" count={6} pressed locale="en" />);

  expect(screen.getByRole("button", { name: "Party popper, 6", pressed: true })).not.toBeNull();
});

test("a chip's count is compact in the locale it is given, and the chip in the reader's direction", () => {
  render(
    <I18nProvider locale="ar-XB">
      <Chip label="Party popper, 12840" glyph="🎉" count={12_840} pressed={false} locale="ja" />
    </I18nProvider>,
  );

  const chip = screen.getByRole("button", { name: "Party popper, 12840" });
  expect(chip.dir).toBe("rtl");
  // The digits are in their own language and never mirrored.
  const count = within(chip).getByText("1.3万");
  expect(count.lang).toBe("ja");
  expect(count.dir).toBe("ltr");
});

test("a disabled chip is disabled and a click does not press it", async () => {
  const user = userEvent.setup();
  let presses = 0;
  render(<Chip label="Party popper, 5" glyph="🎉" count={5} pressed={false} isDisabled onPress={() => presses++} />);
  const chip = screen.getByRole<HTMLButtonElement>("button", { name: "Party popper, 5" });
  expect(chip.disabled).toBe(true);

  await user.click(chip);
  expect(presses).toBe(0);
});

test("a chip row is a group named by its label, in its language, that holds its chips", () => {
  render(
    <ChipRow label="Reactions" lang="en">
      <Chip label="Party popper, 5" glyph="🎉" count={5} pressed={false} locale="en" />
      <Chip label="Red heart, 2" glyph="❤️" count={2} pressed locale="en" />
    </ChipRow>,
  );

  const row = screen.getByRole("group", { name: "Reactions" });
  expect(row.lang).toBe("en");
  expect(within(row).getByRole("button", { name: "Party popper, 5", pressed: false })).not.toBeNull();
  expect(within(row).getByRole("button", { name: "Red heart, 2", pressed: true })).not.toBeNull();
});
