// SPDX-License-Identifier: AGPL-3.0-only

// What the composer's parts are to assistive technology and to their caller: the frame's dialog and how it is left,
// each field's name, state and error, what a press asks of the caller. The keys inside an open menu are the menu's
// (menu.jsdom.test.tsx), and how the parts are laid out is a layout test's.

import { expect, spyOn, test } from "bun:test";
import { Button } from "@my/design-system";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import {
  AttachButton,
  AttachmentList,
  AttachmentRow,
  CharCount,
  ChoiceSelect,
  ComposerFrame,
  ComposerText,
  ToolText,
  UndoNotice,
  WarningField,
  type AttachmentRowProps,
  type ComposerFrameProps,
} from "./composer";

const none = () => undefined;

/** Whether each of `elements` follows the one before it in the document, the order a screen reader reads them in. */
const inOrder = (...elements: Element[]) =>
  elements.every((element, i) => {
    const before = elements[i - 1];
    return before === undefined || (before.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
  });

/** Whether assistive technology is kept from the element: it, or something around it, is inert or hidden from it. */
const outOfReach = (element: Element) => element.closest('[inert], [aria-hidden="true"]') !== null;

/** The composer over a page that has a button of its own, and how it was asked to leave, in order. */
function composer(props: Partial<ComposerFrameProps> = {}) {
  const left: string[] = [];
  render(
    <>
      <button type="button">Behind</button>
      <ComposerFrame
        title="New post"
        closeLabel="Close"
        onClose={() => left.push("close")}
        onDismiss={() => left.push("dismiss")}
        action={<Button>Post</Button>}
        {...props}
      >
        <textarea aria-label="Text" />
      </ComposerFrame>
    </>,
  );
  return left;
}

test("the frame is a dialog named by its title, read in order: the head, what is written, the note, the error, the row", () => {
  render(
    <ComposerFrame
      title="New post"
      closeLabel="Close"
      onClose={none}
      onDismiss={none}
      layer={false}
      headEnd={<Button>Drafts</Button>}
      note={<p>Scheduled for Monday</p>}
      alert={<p role="alert">The post could not be sent.</p>}
      tools={<Button>Add images</Button>}
      count={<span>500 left</span>}
      action={<Button>Post</Button>}
    >
      <textarea aria-label="Text" />
    </ComposerFrame>,
  );
  const dialog = screen.getByRole("dialog", { name: "New post" });
  const part = within(dialog);

  expect(
    inOrder(
      part.getByRole("heading", { name: "New post" }),
      part.getByRole("button", { name: "Drafts" }),
      part.getByRole("button", { name: "Close" }),
      part.getByRole("textbox", { name: "Text" }),
      part.getByText("Scheduled for Monday"),
      part.getByRole("alert"),
      part.getByRole("button", { name: "Add images" }),
      part.getByText("500 left"),
      part.getByRole("button", { name: "Post" }),
    ),
  ).toBe(true);
});

test("Tab goes through the frame in that order", async () => {
  const user = userEvent.setup();
  render(
    <ComposerFrame
      title="New post"
      closeLabel="Close"
      onClose={none}
      onDismiss={none}
      layer={false}
      headEnd={<Button>Drafts</Button>}
      tools={<Button>Add images</Button>}
      action={<Button>Post</Button>}
    >
      <textarea aria-label="Text" />
    </ComposerFrame>,
  );

  const stops: (string | null)[] = [];
  for (let i = 0; i < 5; i += 1) {
    await user.tab();
    stops.push(document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.textContent ?? null);
  }

  expect(stops).toEqual(["Drafts", "Close", "Text", "Add images", "Post"]);
});

test("over the page, the frame holds the focus as it opens and Tab never takes it out", async () => {
  const user = userEvent.setup();
  composer();
  const dialog = await screen.findByRole("dialog", { name: "New post" });

  expect(dialog.contains(document.activeElement)).toBe(true);
  expect(outOfReach(screen.getByText("Behind"))).toBe(true);
  // More presses than it has stops, each way: the focus wraps inside it.
  for (const shift of [false, false, false, false, true, true, true, true]) {
    await user.tab({ shift });
    expect(dialog.contains(document.activeElement)).toBe(true);
  }
});

test("close and Escape say the reader means to leave; a press outside, that they left by accident", async () => {
  const user = userEvent.setup();
  const left = composer();
  const dialog = await screen.findByRole("dialog", { name: "New post" });

  await user.click(within(dialog).getByRole("button", { name: "Close" }));
  await user.keyboard("{Escape}");
  expect(left).toEqual(["close", "close"]);

  await user.click(screen.getByText("Behind"));
  expect(left).toEqual(["close", "close", "dismiss"]);
});

test("while what was written is sent, close is disabled without a busy state of its own, and nothing leaves", async () => {
  const user = userEvent.setup();
  const left = composer({ busy: true });
  const dialog = await screen.findByRole("dialog", { name: "New post" });
  const close = within(dialog).getByRole<HTMLButtonElement>("button", { name: "Close" });

  expect(close.disabled).toBe(true);
  expect(close.hasAttribute("aria-busy")).toBe(false);
  await user.click(close);
  await user.keyboard("{Escape}");
  await user.click(screen.getByText("Behind"));

  expect(left).toEqual([]);
});

test("in the flow of a page the frame leaves the page in reach, and its close still asks to leave", async () => {
  const user = userEvent.setup();
  const left = composer({ layer: false });

  const dialog = screen.getByRole("dialog", { name: "New post" });
  expect(screen.getByRole("button", { name: "Behind" })).not.toBeNull();
  await user.click(within(dialog).getByRole("button", { name: "Close" }));
  expect(left).toEqual(["close"]);
});

test("the count is read as its words, and its digits are kept from a screen reader", () => {
  render(
    <>
      <CharCount left={100} near={20} label="100 characters left" />
      <CharCount left={-3} near={20} label="3 characters over the limit" />
    </>,
  );

  expect(isInaccessible(screen.getByText("100 characters left"))).toBe(false);
  expect(isInaccessible(screen.getByText("3 characters over the limit"))).toBe(false);
  expect(isInaccessible(screen.getByText("100"))).toBe(true);
  expect(isInaccessible(screen.getByText("-3"))).toBe(true);
});

test("the count's id is on its words, so that a field it counts can be described by them; without one they bear none", () => {
  render(
    <>
      <input aria-label="Display name" aria-describedby="name-count" />
      <CharCount id="name-count" left={2} near={10} label="2 characters left" />
      <CharCount left={-3} near={20} label="3 characters over the limit" />
    </>,
  );

  // The words alone, not the digits beside them.
  expect(screen.getAllByRole("textbox", { name: "Display name", description: "2 characters left" })).toHaveLength(1);
  expect(screen.getByText("3 characters over the limit").id).toBe("");
});

test("the text says what is wrong with it under its editor, which that describes; with nothing wrong it says nothing", () => {
  const { rerender } = render(
    <ComposerText error="The text is too long." errorId="text-error" end={<Button>Emoji</Button>}>
      <div role="textbox" aria-label="Text" aria-describedby="text-error" />
    </ComposerText>,
  );

  const box = screen.getByRole("textbox", { name: "Text", description: "The text is too long." });
  expect(inOrder(box, screen.getByText("The text is too long."))).toBe(true);
  expect(screen.getByRole("button", { name: "Emoji" })).not.toBeNull();

  rerender(
    <ComposerText errorId="text-error">
      <div role="textbox" aria-label="Text" aria-describedby="text-error" />
    </ComposerText>,
  );
  expect(screen.getByRole("textbox", { name: "Text", description: "" })).not.toBeNull();
  expect(screen.queryByText("The text is too long.")).toBeNull();
});

test("the content warning says what is wrong with it under its editor, which that describes", () => {
  const { rerender } = render(
    <WarningField error="The warning is too long." errorId="warning-error">
      <div role="textbox" aria-label="Content warning" aria-describedby="warning-error" />
    </WarningField>,
  );

  const box = screen.getByRole("textbox", { name: "Content warning", description: "The warning is too long." });
  expect(inOrder(box, screen.getByText("The warning is too long."))).toBe(true);

  rerender(
    <WarningField errorId="warning-error">
      <div role="textbox" aria-label="Content warning" aria-describedby="warning-error" />
    </WarningField>,
  );
  expect(screen.getByRole("textbox", { name: "Content warning", description: "" })).not.toBeNull();
});

const attachment: AttachmentRowProps = {
  src: "https://m.example/a.webp",
  state: "ready",
  status: "Uploading…",
  description: "",
  onDescription: none,
  descriptionLabel: "Description",
  placeholder: "What it shows",
  removeLabel: "Remove image",
  onRemove: none,
};

test("the images are a list, a row each, and a row's remove asks for it whatever its state", async () => {
  const user = userEvent.setup();
  const removed: string[] = [];
  render(
    <AttachmentList>
      <AttachmentRow {...attachment} removeLabel="Remove image 1" onRemove={() => removed.push("1")} />
      <AttachmentRow
        {...attachment}
        state="uploading"
        removeLabel="Remove image 2"
        onRemove={() => removed.push("2")}
      />
      <AttachmentRow
        {...attachment}
        state="failed"
        status="The upload did not finish."
        removeLabel="Remove image 3"
        onRemove={() => removed.push("3")}
      />
    </AttachmentList>,
  );

  expect(within(screen.getByRole("list")).getAllByRole("listitem")).toHaveLength(3);
  for (const n of ["1", "2", "3"]) await user.click(screen.getByRole("button", { name: `Remove image ${n}` }));
  expect(removed).toEqual(["1", "2", "3"]);
});

test("a ready image has a field named for its description, which tells the caller what is typed", async () => {
  const user = userEvent.setup();
  const typed: string[] = [];
  function Row() {
    const [description, setDescription] = useState("A ridge");
    return (
      <AttachmentRow
        {...attachment}
        description={description}
        onDescription={(value) => {
          typed.push(value);
          setDescription(value);
        }}
      />
    );
  }
  render(<Row />);
  const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Description" });

  expect(field.value).toBe("A ridge");
  expect(field.placeholder).toBe("What it shows");
  expect(field.getAttribute("aria-invalid")).toBeNull();
  await user.type(field, "!");
  expect(typed).toEqual(["A ridge!"]);
  // Its thumbnail is decoration: the description is the field's.
  expect(screen.queryByRole("img")).toBeNull();
});

test("a missing description is a caution that describes the field, and an error where the server asks for one", () => {
  const { rerender } = render(
    <AttachmentRow {...attachment} missing={{ text: "No description yet", required: false }} />,
  );
  const field = screen.getByRole("textbox", { name: "Description", description: "No description yet" });
  expect(field.getAttribute("aria-invalid")).toBeNull();

  rerender(<AttachmentRow {...attachment} missing={{ text: "Describe this image to post it", required: true }} />);
  expect(
    screen
      .getByRole("textbox", { name: "Description", description: "Describe this image to post it" })
      .getAttribute("aria-invalid"),
  ).toBe("true");
});

test("what the server refused of a description stands in the caution's place, and the field is invalid", () => {
  render(
    <AttachmentRow
      {...attachment}
      description="A ridge"
      missing={{ text: "No description yet", required: false }}
      error="The description is too long."
    />,
  );

  const field = screen.getByRole("textbox", { name: "Description", description: "The description is too long." });
  expect(field.getAttribute("aria-invalid")).toBe("true");
  expect(screen.queryByText("No description yet")).toBeNull();
});

test("an image on its way says so as a status, and has no description field yet", () => {
  const { rerender } = render(<AttachmentRow {...attachment} state="uploading" status="Uploading…" />);
  expect(screen.getByRole("status").textContent).toBe("Uploading…");
  expect(screen.queryByRole("textbox")).toBeNull();

  rerender(<AttachmentRow {...attachment} state="processing" status="Processing…" />);
  expect(screen.getByRole("status").textContent).toBe("Processing…");
  expect(screen.queryByRole("textbox")).toBeNull();
});

test("a failed upload says so as a status with its retry, and has no description field", async () => {
  const user = userEvent.setup();
  let retried = 0;
  render(
    <AttachmentRow
      {...attachment}
      src={undefined}
      state="failed"
      status="The upload did not finish."
      retry={{ label: "Try again", onPress: () => retried++ }}
    />,
  );

  const status = screen.getByRole("status");
  expect(within(status).getByText("The upload did not finish.")).not.toBeNull();
  await user.click(within(status).getByRole("button", { name: "Try again" }));
  expect(retried).toBe(1);
  expect(screen.queryByRole("textbox")).toBeNull();
});

/** Presses `button`, which opens the file chooser, and picks `files` in it; gives the chooser the press opened. */
async function choose(user: ReturnType<typeof userEvent.setup>, button: HTMLElement, files: File[]) {
  const opened: HTMLInputElement[] = [];
  const click = spyOn(HTMLInputElement.prototype, "click").mockImplementation(function (this: HTMLInputElement) {
    if (this.type === "file") opened.push(this);
  });
  try {
    await user.click(button);
  } finally {
    click.mockRestore();
  }
  const [chooser] = opened;
  if (chooser === undefined) return undefined;
  await user.upload(chooser, files);
  return chooser;
}

const png = (name: string) => new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], name, { type: "image/png" });

test("the attach button opens the file chooser for several files of its types, and gives the caller what was picked", async () => {
  const user = userEvent.setup();
  const picked: string[][] = [];
  render(
    <AttachButton
      label="Add images"
      accept="image/png,image/jpeg"
      onFiles={(files) => picked.push(files.map((file) => file.name))}
    />,
  );

  const chooser = await choose(user, screen.getByRole("button", { name: "Add images" }), [png("a.png"), png("b.png")]);

  expect(chooser?.accept).toBe("image/png,image/jpeg");
  expect(chooser?.multiple).toBe(true);
  expect(picked).toEqual([["a.png", "b.png"]]);
});

test("the same file may be picked again after it was removed", async () => {
  const user = userEvent.setup();
  const picked: string[][] = [];
  render(
    <AttachButton label="Add images" accept="image/*" onFiles={(files) => picked.push(files.map((f) => f.name))} />,
  );
  const button = screen.getByRole("button", { name: "Add images" });

  await choose(user, button, [png("a.png")]);
  await choose(user, button, [png("a.png")]);

  expect(picked).toEqual([["a.png"], ["a.png"]]);
});

test("a disabled attach button opens no file chooser", async () => {
  const user = userEvent.setup();
  render(<AttachButton label="Add images" accept="image/*" isDisabled onFiles={none} />);
  const button = screen.getByRole<HTMLButtonElement>("button", { name: "Add images" });

  expect(button.disabled).toBe(true);
  expect(await choose(user, button, [png("a.png")])).toBeUndefined();
});

const durations = [
  { id: "1d", icon: "schedule", label: "1 day" },
  { id: "3d", icon: "schedule", label: "3 days" },
] as const;

/** A poll's duration, as the app passes it: the value reads alone and says what it chooses, the label names the menu. */
const duration = {
  label: "Ends in",
  icon: "schedule",
  value: "Ends in 1 day",
  items: durations,
  chosen: "1d",
} as const;

test("a choice control is named by its value, which says what it chooses, and opens a menu named by its label", async () => {
  const user = userEvent.setup();
  render(<ChoiceSelect {...duration} onChoose={none} />);
  const control = screen.getByRole("button", { name: "Ends in 1 day", expanded: false });

  await user.click(control);

  const menu = await screen.findByRole("menu", { name: "Ends in" });
  expect(within(menu).getByRole("menuitemradio", { name: "1 day", checked: true })).not.toBeNull();
  expect(within(menu).getByRole("menuitemradio", { name: "3 days", checked: false })).not.toBeNull();
});

test("a choice tells the caller, closes the menu, and gives the focus back to the control", async () => {
  const user = userEvent.setup();
  const chosen: string[] = [];
  render(<ChoiceSelect {...duration} onChoose={(id) => chosen.push(id)} />);
  const control = screen.getByRole("button", { name: "Ends in 1 day", expanded: false });

  await user.click(control);
  await user.click(await screen.findByRole("menuitemradio", { name: "3 days" }));

  expect(chosen).toEqual(["3d"]);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement === control).toBe(true);
});

test("a tool in words is a button named by its label, and its words, hidden from the name, are in their language", () => {
  render(
    <ToolText label="Language: Japanese" lang="ja" short="JA">
      日本語
    </ToolText>,
  );

  const tool = screen.getByRole("button", { name: "Language: Japanese" });
  const words = within(tool).getByText("日本語");
  expect(isInaccessible(words)).toBe(true);
  expect(words.closest("[lang]")?.getAttribute("lang")).toBe("ja");
});

test("the notice is a status with what it says, and its action when it has one", async () => {
  const user = userEvent.setup();
  let undone = 0;
  const { rerender } = render(
    <UndoNotice action="Undo" onAction={() => undone++} inline>
      Post discarded
    </UndoNotice>,
  );
  const notice = screen.getByRole("status");

  expect(within(notice).getByText("Post discarded")).not.toBeNull();
  await user.click(within(notice).getByRole("button", { name: "Undo" }));
  expect(undone).toBe(1);

  rerender(<UndoNotice inline>Draft saved</UndoNotice>);
  expect(screen.getByRole("status").textContent).toBe("Draft saved");
  expect(screen.queryByRole("button")).toBeNull();
});

test("the notice at the bottom of the page is a status too", () => {
  render(
    <UndoNotice action="Undo" onAction={none}>
      Post discarded
    </UndoNotice>,
  );

  expect(within(screen.getByRole("status")).getByRole("button", { name: "Undo" })).not.toBeNull();
});
