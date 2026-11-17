// SPDX-License-Identifier: AGPL-3.0-only

// What the emoji picker opens in: a popover beside its trigger, a sheet on a phone, and the popover's surface at rest.
// That the picker's field has the focus once it opened is in emoji-picker-overlay.jsdom.test.tsx.

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { EmojiPickerPopover, EmojiPickerSheet, EmojiPickerSurface, LazyEmojiPicker } from "./emoji-picker-overlay";
import { IconButton } from "./icon-button";

const picker = (
  <LazyEmojiPicker
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
  />
);

test("the picker's surface at rest is a dialog named by its label, and the picker shows in it once its module is there", async () => {
  render(<EmojiPickerSurface label="React">{picker}</EmojiPickerSurface>);
  const surface = screen.getByRole("dialog", { name: "React" });

  expect(await within(surface).findByRole("grid", { name: "Emoji" })).not.toBeNull();
  expect(within(surface).getByRole("gridcell", { name: "grinning face" })).not.toBeNull();
});

/** A React button whose popover the test's owner opens and closes, over a page with a button, and the changes asked. */
function ReactPopover({ changes }: { changes: boolean[] }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <EmojiPickerPopover
        label="React"
        open={open}
        onOpenChange={(next) => {
          changes.push(next);
          setOpen(next);
        }}
        trigger={<IconButton icon="react" label="React" />}
      >
        {picker}
      </EmojiPickerPopover>
      <button type="button">Behind</button>
    </>
  );
}

test("the picker's popover is a dialog named by its label, which its trigger opens and Tab never leaves", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<ReactPopover changes={changes} />);
  const trigger = screen.getByRole("button", { name: "React", expanded: false });
  expect(screen.queryByRole("dialog")).toBeNull();

  await user.click(trigger);

  const dialog = await screen.findByRole("dialog", { name: "React" });
  await within(dialog).findByRole("grid", { name: "Emoji" });
  expect(changes).toEqual([true]);
  expect(trigger.getAttribute("aria-expanded")).toBe("true");
  expect(dialog.contains(document.activeElement)).toBe(true);
  // More presses than it has stops, each way: the focus wraps inside it.
  for (const shift of [false, false, false, false, false, true, true, true, true, true, true]) {
    await user.tab({ shift });
    expect(dialog.contains(document.activeElement)).toBe(true);
  }
});

test("Escape closes the picker's popover, and the focus is back on its trigger", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(<ReactPopover changes={changes} />);
  await user.click(screen.getByRole("button", { name: "React" }));
  await screen.findByRole("dialog", { name: "React" });

  await user.keyboard("{Escape}");

  expect(changes).toEqual([true, false]);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "React", expanded: false }));
});

test("a popover with no owner opens and closes by its trigger alone", async () => {
  const user = userEvent.setup();
  render(
    <EmojiPickerPopover label="React" trigger={<IconButton icon="react" label="React" />}>
      {picker}
    </EmojiPickerPopover>,
  );

  await user.click(screen.getByRole("button", { name: "React", expanded: false }));
  await screen.findByRole("dialog", { name: "React" });
  await user.keyboard("{Escape}");

  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "React", expanded: false }));
});

/** A React button that opens the picker's sheet, over a page with a button, and how often the sheet asked to close. */
function ReactSheet({ closes }: { closes: number[] }) {
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
      <button type="button">Behind</button>
      <EmojiPickerSheet
        open={open}
        onClose={() => {
          closes.push(1);
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

test("the picker's sheet is a dialog named by its title, with a close, and Tab never leaves it", async () => {
  const user = userEvent.setup();
  render(<ReactSheet closes={[]} />);
  expect(screen.queryByRole("dialog")).toBeNull();

  await user.click(screen.getByRole("button", { name: "React" }));

  const dialog = await screen.findByRole("dialog", { name: "Pick a reaction" });
  await within(dialog).findByRole("grid", { name: "Emoji" });
  expect(within(dialog).getByRole("heading", { name: "Pick a reaction" })).not.toBeNull();
  expect(within(dialog).getByRole("button", { name: "Close" })).not.toBeNull();
  expect(dialog.contains(document.activeElement)).toBe(true);
  for (const shift of [false, false, false, false, false, false, true, true, true, true, true, true, true]) {
    await user.tab({ shift });
    expect(dialog.contains(document.activeElement)).toBe(true);
  }
});

for (const [how, close] of [
  [
    "its close",
    (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole("button", { name: "Close" })),
  ],
  ["Escape", (user: ReturnType<typeof userEvent.setup>) => user.keyboard("{Escape}")],
] as const)
  test(`${how} closes the picker's sheet, and the focus is back on what opened it`, async () => {
    const user = userEvent.setup();
    const closes: number[] = [];
    render(<ReactSheet closes={closes} />);
    await user.click(screen.getByRole("button", { name: "React" }));
    await screen.findByRole("dialog", { name: "Pick a reaction" });

    await close(user);

    expect(closes).toHaveLength(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "React" }));
  });
