// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so where Edit profile (#3200) puts the
// focus after a Save it did not send, or one the server refused, is decided here (ADR-0171 §4): on the part the
// refusal is about, and on Save where it names none. What the form sends and says is profile-edit.dom.test.tsx.

import { expect, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import {
  alerts,
  bioField,
  chooseFile,
  image,
  imageFile,
  invalid,
  nameField,
  pairField,
  profileForm,
  refusal,
  refusedSave,
  rewrite,
  saveButton,
  type DrawnForm,
} from "../test/profile-form";

/** Waits until `field` has the focus; the page's own description of what has it otherwise. */
const focused = (field: () => HTMLElement, name: string) =>
  waitFor(() => {
    if (document.activeElement !== field()) {
      const has = document.activeElement;
      throw new Error(
        `test: the focus is not on ${name} but on <${has?.tagName.toLowerCase() ?? "nothing"}> "${has?.getAttribute("aria-label") ?? has?.textContent ?? ""}"`,
      );
    }
  });

/** The writes the form sent since `from`. */
const writes = ({ requests }: DrawnForm, from: number) => requests.slice(from).filter((r) => !r.startsWith("GET "));

for (const [name, field, limit] of [
  ["Display name", nameField, 30],
  ["Bio", bioField, 500],
] as const)
  test(`Save with ${name} past its limit sends nothing and moves the focus from Save to ${name}`, async () => {
    const form = await profileForm();
    const before = form.requests.length;
    await rewrite(form.user, field(), "a".repeat(limit + 1));

    await form.user.click(saveButton());

    await focused(field, name);
    expect(writes(form, before)).toEqual([]);
  });

test("Save with both past their limits moves the focus to the first of them, Display name", async () => {
  const form = await profileForm();
  const before = form.requests.length;
  await rewrite(form.user, nameField(), "a".repeat(31));
  await rewrite(form.user, bioField(), "a".repeat(501));

  await form.user.click(saveButton());

  await focused(nameField, "Display name");
  expect(writes(form, before)).toEqual([]);
});

// The member the server's problem names, and the part of the form that takes the focus from Save: the one input, or
// for the fields as a whole their list, which is no control.
for (const [member, name, part] of [
  ["display_name", "Display name", nameField],
  ["note", "Bio", bioField],
  ["fields[0].name", "the label of Field 1", () => pairField(1, "Label")],
  ["fields[1].name", "the label of Field 2", () => pairField(2, "Label")],
  ["fields[1].value", "the content of Field 2", () => pairField(2, "Content")],
  ["fields", "the list of the fields", () => screen.getByRole("list", { name: "Fields" })],
] as const)
  test(`a Save the server refuses for ${member} moves the focus from Save to ${name}`, async () => {
    await refusedSave(refusal(member, "The server does not take this here."));

    await focused(part, name);
  });

const header = "Header image";
const avatar = "Avatar";

for (const [name, member] of [
  [header, "header_media_id"],
  [avatar, "avatar_media_id"],
] as const) {
  test(`an upload of the ${name.toLowerCase()} the server refuses moves the focus from Save to the button that replaces it, in its group`, async () => {
    const form = await profileForm({ uploads: "refused" });
    await chooseFile(form.user, image(name).getByRole("button", { name: "Replace" }), imageFile("dusk.png"));

    await form.user.click(saveButton());

    await focused(() => image(name).getByRole("button", { name: "Replace" }), `Replace of ${name}`);
  });

  test(`the ${name.toLowerCase()} removed, which the server refuses to save, moves the focus from Save to the button that chooses one, in its group`, async () => {
    const form = await profileForm();
    await form.user.click(image(name).getByRole("button", { name: "Remove" }));
    form.fake.script("PATCH /accounts/update_credentials", {
      times: 1,
      fail: refusal(member, "The server does not take this here."),
    });

    await form.user.click(saveButton());

    await focused(() => image(name).getByRole("button", { name: "Choose an image" }), `Choose an image of ${name}`);
  });
}

// A refusal that is about no part of the form: the review's (the fake's own, a problem with no member), and a server
// that fails.
for (const [what, refuse] of [
  ["the server's review refuses", (form: DrawnForm) => form],
  [
    "the server fails",
    (form: DrawnForm) => {
      form.fake.script("PATCH /accounts/update_credentials", { times: 1, fail: 500 });
      return form;
    },
  ],
] as const)
  test(`a Save ${what} without naming a part leaves the focus on Save, and only the bar says so`, async () => {
    const form = refuse(await profileForm(what === "the server fails" ? {} : { profile: { save: "refused" } }));
    await form.user.type(nameField(), "!");

    await form.user.click(saveButton());

    await waitFor(() => {
      if (alerts().length !== 1) throw new Error("test: the bar says nothing of the refused Save yet");
    });
    // Save was busy while the server had the change: the focus did not drop to the page meanwhile, and went nowhere.
    expect(document.activeElement === saveButton()).toBe(true);
    expect(invalid()).toEqual([]);
  });
