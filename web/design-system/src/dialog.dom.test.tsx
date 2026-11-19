// SPDX-License-Identifier: AGPL-3.0-only

import { expect, onTestFinished, test } from "bun:test";
import { FormAlert, TextButton, TextField } from "@my/design-system";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConfirmDialog, PanelDialog, ReauthDialog, type ConfirmDialogProps, type ReauthDialogProps } from "./dialog";

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

test("a confirmation holds its question as a heading and close after it, what follows, then Cancel before the confirmation", async () => {
  const user = userEvent.setup();
  confirmation();
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });

  expect(within(dialog).getByRole("heading", { name: "Delete this post?" })).not.toBeNull();
  expect(within(dialog).getByText("It cannot be undone.")).not.toBeNull();
  await user.tab();
  expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Close" }));
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
  act(() => {
    confirm.focus();
  });
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

// ---- The close of a confirmation ----------------------------------------------------------------------------------

for (const [named, closeLabel, name] of [
  ["the design system's word", undefined, "Close"],
  ["the word it is given", "閉じる", "閉じる"],
] as const)
  test(`the close of a confirmation, named by ${named}, cancels it`, async () => {
    const user = userEvent.setup();
    const answers = confirmation({ closeLabel });
    const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });

    await user.click(within(dialog).getByRole("button", { name }));

    expect(answers).toEqual(["cancel"]);
  });

test("while the confirmed action is in flight, the close of the confirmation is disabled and does not cancel it", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ busy: true });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  const close = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Close" });

  await user.click(close);

  expect(close.disabled).toBe(true);
  expect(answers).toEqual([]);
});

// ---- Authenticating again -----------------------------------------------------------------------------------------

const asks = "To change your password, enter the one you use now.";

/**
 * The confirmation that asks for the password again, over a page that has a button of its own, and what it answered,
 * in order. `again` draws it anew with other props, as its owner does once the proof was sent or refused.
 */
function reauth(props: Partial<ReauthDialogProps> = {}) {
  const answers: string[] = [];
  const dialog = (now: Partial<ReauthDialogProps>) => (
    <>
      <button type="button">Behind</button>
      <ReauthDialog
        title="Confirm it's you"
        about={asks}
        closeLabel="Close"
        cancel="Cancel"
        confirm="Confirm"
        onCancel={() => answers.push("cancel")}
        onConfirm={() => answers.push("confirm")}
        {...now}
      >
        <TextField id="reauth-password" name="password" type="password" label="Password" />
      </ReauthDialog>
    </>
  );
  const { rerender } = render(dialog(props));
  return {
    answers,
    again: (now: Partial<ReauthDialogProps>) => {
      rerender(dialog(now));
    },
  };
}

/** The dialog that asks to authenticate again, once it is open, and its password field. */
async function asking() {
  const dialog = await screen.findByRole("dialog", { name: "Confirm it's you" });
  return { dialog, field: within(dialog).getByLabelText<HTMLInputElement>("Password") };
}

test("authenticating again is a dialog named by its title and described by what it asks, with the focus in its field and the page out of reach", async () => {
  reauth();
  const { dialog, field } = await asking();

  expect(screen.getByRole("dialog", { description: asks }) === dialog).toBe(true);
  expect(within(dialog).getByRole("heading", { name: "Confirm it's you" })).not.toBeNull();
  expect(document.activeElement === field).toBe(true);
  expect(outOfReach(screen.getByText("Behind"))).toBe(true);
});

test("Enter in the field of authenticating again confirms once, and so does a press on its confirmation", async () => {
  const user = userEvent.setup();
  const { answers } = reauth();
  const { dialog } = await asking();

  await user.keyboard("correct horse{Enter}");
  expect(answers).toEqual(["confirm"]);

  await user.click(within(dialog).getByRole("button", { name: "Confirm" }));
  expect(answers).toEqual(["confirm", "confirm"]);
});

for (const [how, cancel] of [
  [
    "its close",
    (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole("button", { name: "Close" })),
  ],
  [
    "its Cancel",
    (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole("button", { name: "Cancel" })),
  ],
  ["Escape", (user: ReturnType<typeof userEvent.setup>) => user.keyboard("{Escape}")],
] as const)
  test(`${how} cancels authenticating again, once, and confirms nothing`, async () => {
    const user = userEvent.setup();
    const { answers } = reauth();
    await asking();

    await cancel(user);

    expect(answers).toEqual(["cancel"]);
  });

test("while the proof is in flight, the confirmation of authenticating again is busy, and no press, Enter or Escape answers", async () => {
  const user = userEvent.setup();
  const { answers } = reauth({ busy: true });
  const { dialog, field } = await asking();

  await user.click(within(dialog).getByRole("button", { name: "Confirm", busy: true }));
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await user.click(within(dialog).getByRole("button", { name: "Close" }));
  act(() => {
    field.focus();
  });
  await user.keyboard("{Enter}");
  await user.keyboard("{Escape}");

  expect(answers).toEqual([]);
  expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Cancel" }).disabled).toBe(true);
  expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Close" }).disabled).toBe(true);
});

test("a proof that is refused shows as an alert the field is described by, and the focus returns to the field", async () => {
  // A button that turns busy, and back, while it has the focus is announced, and React Aria's announcer, which the
  // document keeps from one test file to the next, holds what it said for seven seconds: emptied here, or the next
  // file of the run finds it on the page.
  onTestFinished(() => {
    for (const said of screen.queryAllByRole("log")) said.replaceChildren();
  });
  const user = userEvent.setup();
  const { answers, again } = reauth();
  const { dialog, field } = await asking();
  expect(within(dialog).queryAllByRole("alert")).toHaveLength(0);
  await user.keyboard("wrong");
  await user.click(within(dialog).getByRole("button", { name: "Confirm" }));
  expect(answers).toEqual(["confirm"]);
  expect(document.activeElement === field).toBe(false);

  again({ busy: true });
  again({ error: <FormAlert>That password is not right.</FormAlert> });

  expect(within(dialog).getByRole("alert").textContent).toBe("That password is not right.");
  const describing = (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id));
  expect(describing.map((element) => element?.textContent).join(" ")).toContain("That password is not right.");
  await waitFor(() => {
    if (document.activeElement !== field) throw new Error("test: the focus is not in the field");
  });
});

test("another way to authenticate stands in the dialog, and a press on it neither confirms nor cancels", async () => {
  const user = userEvent.setup();
  let other = 0;
  const { answers } = reauth({ other: <TextButton onPress={() => (other += 1)}>Use a passkey</TextButton> });
  const { dialog } = await asking();

  await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

  expect(other).toBe(1);
  expect(answers).toEqual([]);
});

test("authenticating again by a proof with no field starts with the focus inside it, and its confirmation confirms once", async () => {
  const user = userEvent.setup();
  const answers: string[] = [];
  render(
    <ReauthDialog
      title="Confirm it's you"
      about="To delete your account, confirm with your passkey."
      closeLabel="Close"
      cancel="Cancel"
      confirm="Use a passkey"
      onCancel={() => answers.push("cancel")}
      onConfirm={() => answers.push("confirm")}
    />,
  );
  const dialog = await screen.findByRole("dialog", { name: "Confirm it's you" });
  expect(dialog.contains(document.activeElement)).toBe(true);

  await user.click(within(dialog).getByRole("button", { name: "Use a passkey" }));

  expect(answers).toEqual(["confirm"]);
});

test("authenticating again in the flow of a page is a dialog too, and leaves the page in reach", () => {
  const { answers } = reauth({ layer: false });

  const dialog = screen.getByRole("dialog", { name: "Confirm it's you" });
  expect(within(dialog).getByRole("button", { name: "Confirm" })).not.toBeNull();
  expect(screen.getByRole("button", { name: "Behind" })).not.toBeNull();
  expect(answers).toEqual([]);
});

test("a confirmation with a text to type keeps its confirmation disabled until the field holds the text", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ phrase: "spam_bot@social.example", confirm: "Ban account" });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  const field = within(dialog).getByRole("textbox", { name: "To confirm, enter spam_bot@social.example" });
  const ban = within(dialog).getByRole("button", { name: "Ban account" });
  expect(document.activeElement).toBe(field);
  expect(ban.hasAttribute("disabled")).toBe(true);

  await user.type(field, "spam_bot{Enter}");
  expect(ban.hasAttribute("disabled")).toBe(true);
  expect(answers).toEqual([]);

  // Its edges and full-width forms aside, as a Japanese input method may type it.
  await user.type(field, "@ｓｏｃｉａｌ.example ");
  expect(ban.hasAttribute("disabled")).toBe(false);
  await user.type(field, "{Enter}");
  expect(answers).toEqual(["confirm"]);
});

test("a confirmation with a text to type answers a press only once the text is there, and locks again once it is not", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ phrase: "Moderator", confirm: "Delete role" });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  const field = within(dialog).getByRole("textbox");
  const remove = within(dialog).getByRole("button", { name: "Delete role" });
  // Still described by what follows its question.
  expect(screen.getByRole("alertdialog", { description: (said) => said.includes("It cannot be undone.") })).toBe(
    dialog,
  );

  await user.click(remove);
  expect(answers).toEqual([]);

  await user.type(field, "Moderator");
  await user.type(field, "{Backspace}");
  expect(remove.hasAttribute("disabled")).toBe(true);
  await user.type(field, "{Enter}");
  await user.click(remove);
  expect(answers).toEqual([]);

  await user.type(field, "r");
  await user.click(remove);
  expect(answers).toEqual(["confirm"]);
  // Cancel answers whatever the field holds.
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(answers).toEqual(["confirm", "cancel"]);
});

test("a confirmation with a text to type cancels by its Cancel before anything is typed", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ phrase: "Moderator" });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
  expect(answers).toEqual(["cancel"]);
});

test("a confirmation with a text to type in flight confirms no more", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ phrase: "Moderator", busy: true });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  await user.type(within(dialog).getByRole("textbox"), "Moderator{Enter}");
  expect(answers).toEqual([]);
});

test("a confirmation with a text to type takes the text in any case, as handles and domains are", async () => {
  const user = userEvent.setup();
  const answers = confirmation({ phrase: "Spam_Bot@Social.example", confirm: "Ban account" });
  const dialog = await screen.findByRole("alertdialog", { name: "Delete this post?" });
  await user.type(within(dialog).getByRole("textbox"), "spam_bot@SOCIAL.EXAMPLE{Enter}");
  expect(answers).toEqual(["confirm"]);
});
