// SPDX-License-Identifier: AGPL-3.0-only

// What a settings form of several fields on a line asks of the fields (#3202): a row of a field and its controls; a
// text field whose label is not drawn; a note under a field, which describes it without calling it wrong; a check field
// and a date-time field that cannot be changed now; and the larger target a button and a check field take on a row
// (`touch`), which changes how they look and nothing of what they are. The fields themselves are
// form-page.dom.test.tsx and field.dom.test.tsx, the button button.dom.test.tsx; the sizes are a layout test.

import { expect, test } from "bun:test";
import { Button } from "@my/design-system";
import { act, isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CheckField, DateTimeField, FieldNote, TextField } from "./form-page";
import { SettingsFieldRow, SettingsList } from "./settings";

test("a field's row is a list item with its field first, then its controls, which Tab reaches in that order", async () => {
  const user = userEvent.setup();
  render(
    <SettingsList label="Words">
      <SettingsFieldRow field={<TextField id="word-1" name="keyword" label="Word or phrase 1" labelHidden />}>
        <CheckField id="whole-1" name="whole_word" value="1" label="Whole word" touch />
        <Button touch aria-label="Remove word 1">
          Remove
        </Button>
      </SettingsFieldRow>
      <SettingsFieldRow field={<TextField id="word-2" name="keyword" label="Word or phrase 2" labelHidden />}>
        <CheckField id="whole-2" name="whole_word" value="1" label="Whole word" touch />
      </SettingsFieldRow>
    </SettingsList>,
  );

  const rows = within(screen.getByRole("list", { name: "Words" })).getAllByRole("listitem");
  expect(rows).toHaveLength(2);
  const [first] = rows;
  if (first === undefined) throw new Error("test: the list has no row");
  expect(within(first).getAllByRole("textbox", { name: "Word or phrase 1" })).toHaveLength(1);
  expect(within(first).getAllByRole("checkbox", { name: "Whole word" })).toHaveLength(1);
  expect(within(first).getAllByRole("button", { name: "Remove word 1" })).toHaveLength(1);
  const stops: (string | null | undefined)[] = [];
  for (let i = 0; i < 4; i += 1) {
    await user.tab();
    stops.push(document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.id);
  }
  expect(stops).toEqual(["word-1", "whole-1", "Remove word 1", "word-2"]);
});

test("a text field whose label is not drawn is still named by it, described by its hint and its error, and marked by the error", () => {
  render(
    <>
      <TextField id="drawn" name="a" label="Name" hint="Shown on a folded post." error="Give it a name." />
      <TextField id="hidden" name="b" label="Word or phrase 1" hint="A word." error="A word is too long." labelHidden />
    </>,
  );

  const field = screen.getByRole("textbox", { name: "Word or phrase 1", description: "A word. A word is too long." });
  expect(field.getAttribute("aria-invalid")).toBe("true");
  // The label is in the page for assistive technology, and a press on it still reaches the field.
  expect(isInaccessible(screen.getByText("Word or phrase 1"))).toBe(false);
  expect(screen.getByLabelText("Word or phrase 1") === field).toBe(true);
  // As the field whose label is drawn.
  expect(
    screen.getByRole("textbox", { name: "Name", description: "Shown on a folded post. Give it a name." }),
  ).not.toBeNull();
});

test("a field's note describes the field that names its id, without marking it and without interrupting: it is no alert and no status", () => {
  const { container } = render(
    <>
      <select aria-label="Ends" aria-describedby="ended">
        <option>No change</option>
      </select>
      <FieldNote id="ended">This filter ended and applies nowhere.</FieldNote>
    </>,
  );

  const choice = screen.getByRole("combobox", { name: "Ends", description: "This filter ended and applies nowhere." });
  expect(choice.getAttribute("aria-invalid")).not.toBe("true");
  expect(within(container).queryAllByRole("alert")).toHaveLength(0);
  expect(within(container).queryAllByRole("status")).toHaveLength(0);
  // Its glyph says nothing: the words are the note.
  expect(within(container).queryAllByRole("img")).toHaveLength(0);
  expect(document.getElementById("ended")?.textContent).toBe("This filter ended and applies nowhere.");
});

test("a check field that cannot be changed now is a disabled checkbox with its name, which a press on its label does not turn and Tab passes", async () => {
  const user = userEvent.setup();
  const changes: boolean[] = [];
  render(
    <>
      <CheckField
        id="home"
        name="contexts"
        value="home"
        label="Home and lists"
        defaultChecked
        disabled
        onChange={(checked) => changes.push(checked)}
      />
      <CheckField id="threads" name="contexts" value="thread" label="Threads" />
    </>,
  );
  const box = screen.getByRole<HTMLInputElement>("checkbox", { name: "Home and lists", checked: true });
  expect(box.disabled).toBe(true);

  await user.click(screen.getByText("Home and lists"));
  await user.tab();

  expect(box.checked).toBe(true);
  expect(changes).toEqual([]);
  expect(document.activeElement === screen.getByRole("checkbox", { name: "Threads" })).toBe(true);
});

test("a date-time field that cannot be changed now is disabled, and keeps its name, its value and what describes it", () => {
  const changes: string[] = [];
  render(
    <>
      <p id="zone">In your time zone.</p>
      <DateTimeField
        label="Ends at"
        value="2031-02-03T04:05"
        onChange={(value) => changes.push(value)}
        describedBy="zone"
        disabled
      />
    </>,
  );

  // A field of a date and a time has no role: its label finds it.
  const field = screen.getByLabelText<HTMLInputElement>("Ends at");
  expect(field.type).toBe("datetime-local");
  expect(field.disabled).toBe(true);
  expect(field.value).toBe("2031-02-03T04:05");
  expect(field.getAttribute("aria-describedby")).toBe("zone");
  expect(changes).toEqual([]);
});

// `touch` gives a control of a row a larger target under a finger. It is a look: what the control is to assistive
// technology and to the keyboard stays as without it.
test("a button and a check field with the larger target are what they are without it: their role, name, state and keys", async () => {
  const user = userEvent.setup();
  const pressed: string[] = [];
  const changes: string[] = [];
  /** A button and a check field, with the larger target or without. */
  const drawn = (touch: boolean) => {
    const kind = touch ? "touch" : "plain";
    return (
      <div role="group" aria-label={kind}>
        <Button touch={touch} onPress={() => pressed.push(kind)}>
          Unmute
        </Button>
        <Button touch={touch} isDisabled>
          Add a word
        </Button>
        <Button touch={touch} busy>
          Save
        </Button>
        <CheckField
          id={`whole-${kind}`}
          name="whole_word"
          value="1"
          label="Whole word"
          touch={touch}
          onChange={(checked) => changes.push(`${kind}:${String(checked)}`)}
        />
      </div>
    );
  };
  render(
    <>
      {drawn(false)}
      {drawn(true)}
    </>,
  );
  /** What assistive technology gets of each control of a group, in the page's order. */
  const heard = (kind: string) => {
    const group = within(screen.getByRole("group", { name: kind }));
    return [
      ...group.getAllByRole<HTMLButtonElement>("button").map((button) => ({
        role: "button",
        name: button.textContent,
        disabled: button.disabled,
        busy: button.getAttribute("aria-busy"),
        type: button.type,
      })),
      ...group.getAllByRole<HTMLInputElement>("checkbox", { name: "Whole word" }).map((box) => ({
        role: "checkbox",
        name: "Whole word",
        disabled: box.disabled,
        checked: box.checked,
      })),
    ];
  };
  expect(heard("touch")).toEqual(heard("plain"));
  expect(heard("touch")).toHaveLength(4);

  for (const kind of ["plain", "touch"]) {
    const group = within(screen.getByRole("group", { name: kind }));
    await user.click(group.getByRole("button", { name: "Unmute" }));
    await user.click(group.getByText("Whole word"));
    act(() => {
      group.getByRole<HTMLInputElement>("checkbox").focus();
    });
    await user.keyboard(" ");
  }

  expect(pressed).toEqual(["plain", "touch"]);
  expect(changes).toEqual(["plain:true", "plain:false", "touch:true", "touch:false"]);
});
