// SPDX-License-Identifier: AGPL-3.0-only

// The field of a picture a reader picks (#3200), an avatar's or a header's: what it is to assistive technology (a group
// named by its label, which its hint and what is wrong with the picture describe, the picture itself a decoration),
// what its buttons do (the first opens the browser's file chooser and reports the file picked there, Remove reports
// that it was pressed), and that neither does anything while the field is disabled. How the picture is drawn in each
// shape is the story's (Picture field) and the layout tests'.

import { expect, spyOn, test } from "bun:test";
import { cleanup, isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { PictureField, type PictureFieldProps } from "./picture-field";

/** A header's field with what `props` give it, and what it reported: the files picked and the presses of Remove. */
function header(props: Partial<PictureFieldProps> = {}) {
  const picked: File[] = [];
  render(
    <PictureField
      id="header"
      label="Header image"
      shape="banner"
      name="Yuki"
      choose="Choose an image"
      onFile={(file) => picked.push(file)}
      {...props}
    />,
  );
  const group = screen.getByRole("group", { name: "Header image" });
  return { picked, group, part: within(group) };
}

/** What describes `element`, part by part. */
const describedBy = (element: HTMLElement) =>
  (element.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");

/** The names of the buttons in `group`, in their order. */
const buttons = (group: HTMLElement) =>
  within(group)
    .getAllByRole("button")
    .map((button) => button.textContent);

/** Presses `button` and returns the file choosers the press opened: no simulation opens one. */
async function chooserOf(user: UserEvent, button: HTMLElement): Promise<HTMLInputElement[]> {
  const opened: HTMLInputElement[] = [];
  const click = spyOn(HTMLInputElement.prototype, "click").mockImplementation(function (this: HTMLInputElement) {
    if (this.type === "file") opened.push(this);
  });
  try {
    await user.click(button);
  } finally {
    click.mockRestore();
  }
  return opened;
}

const png = (name: string) => new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], name, { type: "image/png" });

test("a picture field with no picture is a group named by its label, with the one button that chooses a picture", () => {
  const { group, part } = header();

  expect(buttons(group)).toEqual(["Choose an image"]);
  expect(part.queryAllByRole("presentation")).toHaveLength(0);
  expect(part.queryAllByRole("img")).toHaveLength(0);
  // Nothing describes it while it has no hint and nothing is wrong.
  expect(describedBy(group)).toEqual([]);
});

test("a picture field with a picture shows it as a decoration, and Remove stands after the button that replaces it", () => {
  const { group, part } = header({
    src: "https://media.example/header.png",
    choose: "Replace",
    remove: { label: "Remove", onPress: () => undefined },
  });

  // The label says what the picture is: the picture itself has no name of its own.
  expect(part.getAllByRole<HTMLImageElement>("presentation").map((picture) => picture.src)).toEqual([
    "https://media.example/header.png",
  ]);
  expect(part.queryAllByRole("img")).toHaveLength(0);
  expect(buttons(group)).toEqual(["Replace", "Remove"]);
});

test("the hint, then what is wrong with the picture, describe the group", () => {
  const { group } = header({
    src: "https://media.example/header.png",
    hint: "Shown across the top of your profile.",
    error: "This image could not be used. Choose another one.",
  });

  expect(describedBy(group)).toEqual([
    "Shown across the top of your profile.",
    "This image could not be used. Choose another one.",
  ]);
});

test("a hint alone describes the group, and so does what is wrong alone", () => {
  const hinted = header({ hint: "Shown across the top of your profile." });
  expect(describedBy(hinted.group)).toEqual(["Shown across the top of your profile."]);
  cleanup();

  const refused = header({ error: "This image could not be used. Choose another one." });
  expect(describedBy(refused.group)).toEqual(["This image could not be used. Choose another one."]);
});

test("the button opens the browser's file chooser for images, and the file picked there is reported once", async () => {
  const user = userEvent.setup();
  const { picked, part } = header();

  const [chooser, ...others] = await chooserOf(user, part.getByRole("button", { name: "Choose an image" }));
  if (chooser === undefined) throw new Error("test: the press opened no file chooser");
  expect(others).toHaveLength(0);
  // The chooser is the browser's: the page shows no control of its own for it.
  expect(isInaccessible(chooser)).toBe(true);
  expect(chooser.accept).toBe("image/*");
  expect(chooser.multiple).toBe(false);
  expect(picked).toEqual([]);

  const file = png("dusk.png");
  await user.upload(chooser, file);

  expect(picked).toEqual([file]);
});

test("the button that replaces a picture opens the same chooser, and Remove reports its press and opens none", async () => {
  const user = userEvent.setup();
  let removed = 0;
  const { picked, part } = header({
    src: "https://media.example/header.png",
    choose: "Replace",
    remove: { label: "Remove", onPress: () => (removed += 1) },
  });

  expect(await chooserOf(user, part.getByRole("button", { name: "Remove" }))).toHaveLength(0);
  expect(removed).toBe(1);

  const [chooser] = await chooserOf(user, part.getByRole("button", { name: "Replace" }));
  if (chooser === undefined) throw new Error("test: the press opened no file chooser");
  const file = png("snow.png");
  await user.upload(chooser, file);

  expect(picked).toEqual([file]);
  expect(removed).toBe(1);
});

test("a disabled picture field keeps its picture and its buttons, which do nothing", async () => {
  const user = userEvent.setup();
  let removed = 0;
  const { picked, part } = header({
    src: "https://media.example/header.png",
    choose: "Replace",
    remove: { label: "Remove", onPress: () => (removed += 1) },
    isDisabled: true,
  });
  const replace = part.getByRole<HTMLButtonElement>("button", { name: "Replace" });
  const remove = part.getByRole<HTMLButtonElement>("button", { name: "Remove" });

  expect([replace.disabled, remove.disabled]).toEqual([true, true]);
  expect(part.getAllByRole("presentation")).toHaveLength(1);
  expect(await chooserOf(user, replace)).toHaveLength(0);
  expect(await chooserOf(user, remove)).toHaveLength(0);
  expect(removed).toBe(0);
  expect(picked).toEqual([]);
});
