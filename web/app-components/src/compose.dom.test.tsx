// SPDX-License-Identifier: AGPL-3.0-only

// What the composer's parts that know a post are to assistive technology and to their caller: the post replied to and
// the quoted post above the text, and the poll's card with its options as they are written.

import { expect, test } from "bun:test";
import { Button, PollSettings } from "@my/design-system";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { ComposerQuote, ComposerReply, PollCard, PollOption, PollOptions } from "./compose";

const none = () => undefined;

/** Whether each of `elements` follows the one before it in the document, the order a screen reader reads them in. */
const inOrder = (...elements: Element[]) =>
  elements.every((element, i) => {
    const before = elements[i - 1];
    return before === undefined || (before.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
  });

test("the post replied to shows who wrote it, its text, and the note that says what it is", () => {
  render(
    <ComposerReply byline={<span>Lena Novak</span>} note="The post you reply to">
      Quiet morning.
    </ComposerReply>,
  );

  for (const words of ["Lena Novak", "Quiet morning.", "The post you reply to"])
    expect(isInaccessible(screen.getByText(words))).toBe(false);
});

test("the quoted post shows as given, with a remove named by its label", async () => {
  const user = userEvent.setup();
  let removed = 0;
  render(
    <ComposerQuote removeLabel="Remove the quoted post" onRemove={() => removed++}>
      <p>A small server.</p>
    </ComposerQuote>,
  );

  expect(screen.getByText("A small server.")).not.toBeNull();
  await user.click(screen.getByRole("button", { name: "Remove the quoted post" }));
  expect(removed).toBe(1);
});

test("the poll is a group named by its title, with a remove named by its label", async () => {
  const user = userEvent.setup();
  let removed = 0;
  render(
    <PollCard title="Poll" removeLabel="Remove the poll" onRemove={() => removed++}>
      <PollOptions>
        <PollOption value="" onChange={none} label="Option 1" multiple={false} maxLength={50} />
      </PollOptions>
      <PollSettings>
        <Button>1 day</Button>
      </PollSettings>
    </PollCard>,
  );
  const poll = screen.getByRole("group", { name: "Poll" });

  expect(within(poll).getByRole("textbox", { name: "Option 1" })).not.toBeNull();
  expect(within(poll).getByRole("button", { name: "1 day" })).not.toBeNull();
  await user.click(within(poll).getByRole("button", { name: "Remove the poll" }));
  expect(removed).toBe(1);
});

test("an option is a field named by its label that takes no more than its limit and tells the caller what is typed", async () => {
  const user = userEvent.setup();
  const typed: string[] = [];
  function Option() {
    const [value, setValue] = useState("Ry");
    return (
      <PollOption
        value={value}
        onChange={(next) => {
          typed.push(next);
          setValue(next);
        }}
        label="Option 1"
        multiple={false}
        maxLength={4}
      />
    );
  }
  render(<Option />);
  const field = screen.getByRole<HTMLInputElement>("textbox", { name: "Option 1" });

  expect(field.value).toBe("Ry");
  expect(field.getAttribute("aria-invalid")).toBeNull();
  await user.type(field, "e!!!");

  expect(typed).toEqual(["Rye", "Rye!"]);
  expect(field.value).toBe("Rye!");
  // Without a remove of its own, the option has none.
  expect(screen.queryByRole("button")).toBeNull();
});

test("an option says what is wrong with it under it, and has its remove where it may go", async () => {
  const user = userEvent.setup();
  let removed = 0;
  render(
    <PollOptions>
      <PollOption
        value=""
        onChange={none}
        label="Option 3"
        multiple
        maxLength={50}
        remove={{ label: "Remove option 3", onPress: () => removed++ }}
        error="Write this option or remove it."
      />
    </PollOptions>,
  );

  const field = screen.getByRole("textbox", { name: "Option 3", description: "Write this option or remove it." });
  expect(field.getAttribute("aria-invalid")).toBe("true");
  expect(inOrder(field, screen.getByText("Write this option or remove it."))).toBe(true);
  await user.click(screen.getByRole("button", { name: "Remove option 3" }));
  expect(removed).toBe(1);
});

test("the options' last line adds one, where one may be added", async () => {
  const user = userEvent.setup();
  let added = 0;
  const { rerender } = render(
    <PollOptions add={{ label: "Add an option", onPress: () => added++ }}>
      <PollOption value="" onChange={none} label="Option 1" multiple={false} maxLength={50} />
    </PollOptions>,
  );
  const add = screen.getByRole("button", { name: "Add an option" });

  expect(inOrder(screen.getByRole("textbox", { name: "Option 1" }), add)).toBe(true);
  await user.click(add);
  expect(added).toBe(1);

  rerender(
    <PollOptions>
      <PollOption value="" onChange={none} label="Option 1" multiple={false} maxLength={50} />
    </PollOptions>,
  );
  expect(screen.queryByRole("button")).toBeNull();
});
