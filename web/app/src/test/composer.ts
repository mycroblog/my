// SPDX-License-Identifier: AGPL-3.0-only

// The composer, for the component tests of the app's routes: the dialog a press on New post opens, and its text, an
// editor whose content the test reads as the composer sends it. No simulation types into the editor as a browser does
// (a key pressed in an empty box writes nothing there), so a test writes by pasting, which the editor takes from the
// event's own data, and the editing a keyboard does stays with the tests in a browser.

import { screen, waitFor, within } from "@testing-library/react";
import type { TestApp } from "./app";
import { rail } from "./viewport";

/** The composer open over the screen: its dialog, and its text once the editor has come. */
export type Composer = { dialog: HTMLElement; text: HTMLElement };

/** The composer whose dialog is named `name` ("New post", "Reply", "Edit post"), once its text is in place. */
export async function composerOf(name: string): Promise<Composer> {
  const dialog = await screen.findByRole("dialog", { name });
  return { dialog, text: await within(dialog).findByRole("textbox", { name: "Text" }) };
}

/** Opens the composer of a new post from the rail's New post. */
export async function newPost(app: TestApp): Promise<Composer> {
  await app.user.click(within(rail()).getByRole("button", { name: "New post" }));
  return composerOf("New post");
}

/**
 * What `box`, a text box of the editor, holds as the composer sends it: its lines joined by "\n", and a custom emoji,
 * which the editor shows as its image, as its `:shortcode:`.
 */
export function heldBy(box: HTMLElement): string {
  const read = (node: Node): string => {
    if (node instanceof Text) return node.data.replaceAll("​", "");
    if (!(node instanceof Element) || node.hasAttribute("data-lexical-managed-linebreak")) return "";
    if (node instanceof HTMLBRElement) return "\n";
    if (node instanceof HTMLImageElement) return node.alt;
    return Array.from(node.childNodes, read).join("");
  };
  return Array.from(box.children, read).join("\n");
}

/** Pastes `text` at the caret of `box`, as Ctrl+V does with plain text on the clipboard. */
export async function paste(app: TestApp, box: HTMLElement, text: string): Promise<void> {
  if (document.activeElement !== box) await app.user.click(box);
  await app.user.paste(text);
}

/** Writes `text` in place of what `box` holds: pasted, since the editor takes no typed key on a simulation. */
export async function replace(app: TestApp, box: HTMLElement, text: string): Promise<void> {
  await app.user.click(box);
  await app.user.keyboard("{Control>}a{/Control}{Delete}");
  if (text !== "") await app.user.paste(text);
}

/**
 * The composer whose dialog is named `name` once its text holds `text`: opening a draft in the composer puts a new
 * one in its place, so the test finds it again.
 */
export const composerHolding = (name: string, text: string): Promise<Composer> =>
  waitFor(() => {
    const dialog = screen.getByRole("dialog", { name });
    const box = within(dialog).getByRole("textbox", { name: "Text" });
    if (heldBy(box) !== text) throw new Error(`test: the composer ${name} holds "${heldBy(box)}", not "${text}"`);
    return { dialog, text: box };
  });
