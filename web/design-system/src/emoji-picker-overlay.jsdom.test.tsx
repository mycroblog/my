// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: on happy-dom the dialog of a popover or a sheet keeps the focus that the field of the picker in it asked
// for, so where the focus is once the picker opened is not decided there (ADR-0171 §4).

import { expect, test } from "bun:test";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { EmojiPicker } from "./emoji-picker";
import { EmojiPickerPopover, EmojiPickerSheet } from "./emoji-picker-overlay";
import { IconButton } from "./icon-button";

const picker = (
  <EmojiPicker
    sections={[
      {
        id: "faces",
        label: "Faces",
        icon: "emojiSmileys",
        emojis: [{ key: "😀", name: "grinning face", text: "😀" }],
      },
    ]}
    search={() => []}
    labels={{ tone: "Skin tone", tones: ["Default", "Light", "Medium-light", "Medium", "Medium-dark", "Dark"] }}
    onSelect={() => undefined}
    autoFocus
  />
);

/** The focus is back on the React button: on jsdom it arrives a frame after the close. */
async function focusIsBack() {
  await waitFor(
    () => {
      // A cheap look while it waits: a query that finds nothing describes the whole page each time it fails.
      if (document.activeElement?.getAttribute("aria-label") !== "React") throw new Error("the focus is not back");
    },
    { interval: 5 },
  );
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "React" }));
}

test("the picker's field has the focus once its popover opened from the keyboard, and Escape gives it back to the trigger", async () => {
  const user = userEvent.setup();
  render(
    <EmojiPickerPopover label="React" trigger={<IconButton icon="react" label="React" />}>
      {picker}
    </EmojiPickerPopover>,
  );
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "React", expanded: false }));

  await user.keyboard("{Enter}");

  const dialog = await screen.findByRole("dialog", { name: "React" });
  expect(document.activeElement).toBe(within(dialog).getByRole("searchbox", { name: "Search emoji" }));

  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).toBeNull();
  await focusIsBack();
});

/** A React button that opens the picker's sheet. */
function ReactSheet() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <IconButton
        icon="react"
        label="React"
        onPress={() => {
          setOpen(true);
        }}
      />
      <EmojiPickerSheet
        open={open}
        onClose={() => {
          setOpen(false);
        }}
        title="Pick a reaction"
        closeLabel="Close"
      >
        {picker}
      </EmojiPickerSheet>
    </>
  );
}

test("the picker's field has the focus once its sheet opened from the keyboard, and Escape gives it back to what opened it", async () => {
  const user = userEvent.setup();
  render(<ReactSheet />);
  await user.tab();

  await user.keyboard("{Enter}");

  const dialog = await screen.findByRole("dialog", { name: "Pick a reaction" });
  expect(document.activeElement).toBe(within(dialog).getByRole("searchbox", { name: "Search emoji" }));

  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).toBeNull();
  await focusIsBack();
});
