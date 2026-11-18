// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so it cannot tell an alert that can
// take the focus from one that cannot, nor whether the part a refusal names (#3200), a list of pairs among them, can
// take the focus a form gives it (ADR-0171 §4).

import { expect, test } from "bun:test";
import { PairField, PairList, PictureField } from "@my/design-system";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "./button";
import { FormActions, FormAlert, TextField, focusRefusedField } from "./form-page";

test("a form's alert takes the focus a page gives it, and Tab goes on to the button after it", async () => {
  const user = userEvent.setup();
  render(
    <FormActions>
      <FormAlert focus>The password is wrong.</FormAlert>
      <Button kind="primary" type="submit">
        Sign in
      </Button>
    </FormActions>,
  );
  const alert = screen.getByRole("alert");

  alert.focus();
  expect(document.activeElement).toBe(alert);
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Sign in" }));
});

test("a form's alert is no stop of Tab by itself", async () => {
  const user = userEvent.setup();
  render(
    <FormActions>
      <FormAlert focus>The password is wrong.</FormAlert>
      <Button kind="primary" type="submit">
        Sign in
      </Button>
    </FormActions>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Sign in" }));
});

/**
 * A form with a part of each kind a server may refuse, and Save: the function returned calls `focusRefusedField(id)`
 * while Save has the focus, as a form does once the server answered its Save.
 */
function refusedForm() {
  render(
    <>
      <TextField id="name" name="name" label="Display name" error="This name is too long." />
      <PictureField
        id="header"
        label="Header image"
        shape="banner"
        name="Yuki"
        choose="Choose an image"
        onFile={() => undefined}
        error="This image could not be used."
      />
      <PairList id="pairs" label="Fields" error="You can have at most 4.">
        <PairField
          id="pair"
          legend="Field 1"
          nameLabel="Label"
          name="Photos"
          onName={() => undefined}
          valueLabel="Content"
          value="https://yuki.photos.example"
          onValue={() => undefined}
          maxLength={255}
          remove={{ label: "Remove field 1", onPress: () => undefined }}
        />
      </PairList>
      <Button>Save</Button>
    </>,
  );
  const save = screen.getByRole("button", { name: "Save" });
  return (id: string) => {
    act(() => {
      save.focus();
      focusRefusedField(id);
    });
  };
}

/** Whether `part` has the focus. */
const focused = (part: HTMLElement) => document.activeElement === part;

// The ids a form may name: a text field's own, a picture field's for its button that picks one (the group around it
// takes no focus), a pair's for its label and the same with -value for its content.
for (const [id, what, part] of [
  ["name", "a text field", () => screen.getByRole("textbox", { name: "Display name" })],
  [
    "header",
    "the button that picks a picture",
    () => within(screen.getByRole("group", { name: "Header image" })).getByRole("button", { name: "Choose an image" }),
  ],
  ["pair", "a pair's label", () => screen.getByRole("textbox", { name: "Label" })],
  ["pair-value", "a pair's content", () => screen.getByRole("textbox", { name: "Content" })],
] as const)
  test(`focusRefusedField moves the focus from Save to ${what}, by the id "${id}"`, () => {
    const refuse = refusedForm();
    expect(focused(part())).toBe(false);

    refuse(id);

    expect(focused(part())).toBe(true);
  });

test("focusRefusedField moves the focus to the list of pairs, which what is wrong with it describes and which is no stop of Tab", async () => {
  const user = userEvent.setup();
  const refuse = refusedForm();
  const list = screen.getByRole("list", { name: "Fields" });

  refuse("pairs");

  expect(focused(list)).toBe(true);
  expect(
    (list.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent),
  ).toEqual(["You can have at most 4."]);
  // From the list, Tab goes on into its first pair.
  await user.tab();
  expect(focused(screen.getByRole("textbox", { name: "Label" }))).toBe(true);
  // And back from the first pair, Tab passes the list: the stop before it is the picture's button.
  await user.tab({ shift: true });
  expect(focused(screen.getByRole("button", { name: "Choose an image" }))).toBe(true);
});

test("focusRefusedField leaves the focus on Save when no part bears the id", () => {
  const refuse = refusedForm();

  refuse("nothing");

  expect(focused(screen.getByRole("button", { name: "Save" }))).toBe(true);
});
