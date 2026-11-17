// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog, PanelDialog, type ConfirmDialogProps } from "./dialog";

/** A confirmation over a page that has a button of its own, and what it answered, in order. */
function confirmation(props: Partial<ConfirmDialogProps> = {}) {
  const answers: string[] = [];
  render(
    <>
      <button type="button">Behind</button>
      <ConfirmDialog
        title="Delete this post?"
        cancel="Cancel"
        confirm="Delete"
        onCancel={() => answers.push("cancel")}
        onConfirm={() => answers.push("confirm")}
        {...props}
      >
        It cannot be undone.
      </ConfirmDialog>
    </>,
  );
  return answers;
}

/** Whether assistive technology is kept from the element: it, or something around it, is inert or hidden from it. */
const outOfReach = (element: Element) => element.closest('[inert], [aria-hidden="true"]') !== null;

test("a confirmation is an alertdialog named by its title, and Tab never takes the focus out of it", async () => {
  const user = userEvent.setup();
  confirmation();
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  expect(dialog.contains(document.activeElement)).toBe(true);

  // More presses than it has buttons, each way: the focus wraps inside it.
  for (const shift of [false, false, false, true, true, true]) {
    await user.tab({ shift });
    expect(dialog.contains(document.activeElement)).toBe(true);
  }
});

test("a confirmation holds its question as a heading, what follows, then Cancel before the confirmation", async () => {
  const user = userEvent.setup();
  confirmation();
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });

  expect(within(dialog).getByRole("heading", { name: "Delete this post?" })).not.toBeNull();
  expect(within(dialog).getByText("It cannot be undone.")).not.toBeNull();
  await user.tab();
  expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Cancel" }));
  await user.tab();
  expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Delete" }));
});

// WAI-ARIA's alert dialog pattern: aria-describedby names the element that holds the alert's message.
test("a confirmation is described by what follows its question", async () => {
  confirmation();
  await screen.findByRole("alertdialog", { name: "Delete this post?" });

  expect(screen.getByRole("alertdialog", { description: "It cannot be undone." })).not.toBeNull();
});

test("a confirmation over the page keeps assistive technology from the page behind it", async () => {
  confirmation();
  await screen.findByRole("alertdialog", { name: "Delete this post?" });

  expect(outOfReach(screen.getByText("Behind"))).toBe(true);
});

test("Escape in a confirmation cancels it", async () => {
  const user = userEvent.setup();
  const answers = confirmation();
  await screen.findByRole("alertdialog", { name: "Delete this post?" });

  await user.keyboard("{Escape}");

  expect(answers).toEqual(["cancel"]);
});

test("the buttons of a confirmation answer it, by a press and by Enter", async () => {
  const user = userEvent.setup();
  const answers = confirmation();
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });

  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  await user.keyboard("{Enter}");

  expect(answers).toEqual(["cancel", "confirm", "confirm"]);
});

test("while the confirmed action is in flight the confirmation is busy, and neither button answers", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ busy: true });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });

  const confirm = within(dialog).getByRole("button", { name: "Delete", busy: true });
  await user.click(confirm);
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  confirm.focus();
  await user.keyboard("{Enter}");
  await user.keyboard(" ");

  expect(answers).toEqual([]);
  expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(true);
});

test("a confirmation that is not busy has no busy button", async () => {
  confirmation();
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });

  expect(within(dialog).queryByRole("button", { busy: true })).toBeNull();
});

// Its Cancel is disabled while the action is in flight, and "the dialog stays": Escape still cancels it.
test("while the confirmed action is in flight, Escape does not cancel the confirmation", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ busy: true });
  await screen.findByRole("alertdialog", { name: "Delete this post?" });

  await user.keyboard("{Escape}");

  expect(answers).toEqual([]);
});

test("a confirmation in the flow of a page is an alertdialog too, and leaves the page in reach", () => {
  const answers = confirmation({ layer: false });

  const dialog = screen.getByRole("alertdialog", { name: "Delete this post?" });
  expect(within(dialog).getByRole("button", { name: "Cancel" })).not.toBeNull();
  expect(within(dialog).getByRole("button", { name: "Delete" })).not.toBeNull();
  expect(screen.getByRole("button", { name: "Behind" })).not.toBeNull();
  expect(answers).toEqual([]);
});

/** A panel over a page that has a button of its own, with a link in what it shows, and how often it asked to close. */
function panel({ layer }: { layer?: boolean } = {}) {
  const closes: number[] = [];
  render(
    <>
      <button type="button">Behind</button>
      <PanelDialog title="Edit history" closeLabel="Close" onClose={() => closes.push(1)} layer={layer}>
        <a href="/posts/1/history/2">Yesterday</a>
      </PanelDialog>
    </>,
  );
  return closes;
}

test("a panel is a dialog named by its title, which keeps the focus and assistive technology inside it", async () => {
  const user = userEvent.setup();
  panel();
  const dialog = await screen.findByRole("dialog", { name: "Edit history" });

  expect(within(dialog).getByRole("heading", { name: "Edit history" })).not.toBeNull();
  expect(dialog.contains(document.activeElement)).toBe(true);
  expect(outOfReach(screen.getByText("Behind"))).toBe(true);
  // Its close, then what it shows; more presses than it has stops, each way: the focus wraps inside it.
  await user.tab();
  expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Close" }));
  await user.tab();
  expect(document.activeElement).toBe(within(dialog).getByRole("link", { name: "Yesterday" }));
  for (const shift of [false, false, true, true, true]) {
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
  ["a press outside it", (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByText("Behind"))],
] as const)
  test(`${how} asks a panel to close, once`, async () => {
    const user = userEvent.setup();
    const closes = panel();
    await screen.findByRole("dialog", { name: "Edit history" });

    await close(user);

    expect(closes).toHaveLength(1);
  });

test("a panel in the flow of a page is a dialog too, and leaves the page in reach", async () => {
  const user = userEvent.setup();
  const closes = panel({ layer: false });

  const dialog = screen.getByRole("dialog", { name: "Edit history" });
  expect(screen.getByRole("button", { name: "Behind" })).not.toBeNull();
  await user.click(within(dialog).getByRole("button", { name: "Close" }));
  expect(closes).toHaveLength(1);
});
