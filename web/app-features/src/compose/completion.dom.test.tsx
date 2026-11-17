// SPDX-License-Identifier: AGPL-3.0-only

// A writer completes an account after `@`, a hashtag after `#` and an emoji after `:` in the composer, by keyboard and
// by pointer (FR-119, ADR-0092): the text keeps the focus, the list is announced, and what is picked is written with a
// space after it. The list's place under the caret's line is measured, which no simulation does: it stays unseen
// there, so these tests find it from the text that controls it and not as a reader sees it, and write by pasting.
// What an input method does to the list stays with the tests in a browser.

import { afterEach, expect, test } from "bun:test";
import { defaultComposeSettings } from "@my/app-data";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { fakeData, pasteInto, renderFeature } from "../test/feature";
import { ComposerView } from "./composer";
import { emptyDraft } from "./model";
import { useComposer } from "./use-composer";

const none = () => undefined;

afterEach(() => {
  // The hashtags posted lately are kept on the device.
  localStorage.clear();
});

/** The composer of a new post, alone in the document: its dialog, its text, and the reader's hands. */
async function composing() {
  cleanup();
  const made = fakeData();
  function Composing() {
    const c = useComposer(
      { ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"), text: "" },
      { onPosted: none, onKept: none },
    );
    return <ComposerView c={c} layer={false} onClose={none} onDismiss={none} />;
  }
  const user = renderFeature(made.data, <Composing />);
  const dialog = screen.getByRole("dialog", { name: "New post" });
  const text = await within(dialog).findByRole("textbox", { name: "Text" });
  return { ...made, user, dialog, text };
}

/** What `box` holds as the composer sends it: its lines joined by "\n", a custom emoji, an image, as its `:shortcode:`. */
function heldBy(box: HTMLElement): string {
  const read = (node: Node): string => {
    if (node instanceof Text) return node.data.replaceAll("​", "");
    if (!(node instanceof Element) || node.hasAttribute("data-lexical-managed-linebreak")) return "";
    if (node instanceof HTMLBRElement) return "\n";
    if (node instanceof HTMLImageElement) return node.alt;
    return Array.from(node.childNodes, read).join("");
  };
  return Array.from(box.children, read).join("\n");
}

/** The name of a suggestion: its label, else its words. */
const named = (option: Element) => option.getAttribute("aria-label") ?? option.textContent;

/** The list of suggestions the text controls, named `kind` ("Accounts", "Hashtags", "Emoji"); none while it is closed. */
function listOf(text: HTMLElement, kind: string): HTMLElement | null {
  const list = document.getElementById(text.getAttribute("aria-controls") ?? "");
  return list?.role === "listbox" && list.getAttribute("aria-label") === kind ? list : null;
}

/** The suggestions of `kind` the text controls, in their order; none while the list is closed. */
const suggested = (text: HTMLElement, kind: string): string[] =>
  Array.from(listOf(text, kind)?.querySelectorAll("[role=option]") ?? [], named);

/** The text's active suggestion, the option its `aria-activedescendant` names; undefined while it names none. */
function active(text: HTMLElement): string | undefined {
  const option = document.getElementById(text.getAttribute("aria-activedescendant") ?? "");
  return option === null ? undefined : named(option);
}

/**
 * Waits until the suggestions of `kind` start with `options`, in this order, the first of them active, and returns
 * them all.
 */
async function suggests(text: HTMLElement, kind: string, options: readonly (string | RegExp)[]): Promise<string[]> {
  return waitFor(() => {
    const shown = suggested(text, kind);
    const same = options.every((option, i) =>
      typeof option === "string" ? shown[i] === option : option.test(shown[i] ?? ""),
    );
    if (!same || active(text) !== shown[0])
      throw new Error(
        `test: the ${kind} suggested are ${JSON.stringify(shown)}, the active one ${String(active(text))}`,
      );
    return shown;
  });
}

/** The focus is in `text`, as a boolean: a failed comparison of two elements would describe the whole page. */
const focusedIn = (text: HTMLElement) => document.activeElement === text;

test("a writer completes an account by keyboard: the followed first, the list announced, the caret after it", async () => {
  const { user, dialog, text } = await composing();
  await user.click(text);
  await pasteInto(user, text, "Hi @m");

  // Marco and Mio by the start of their names, Sam by the m in sam; Sam, whom the writer follows, comes first.
  expect(await suggests(text, "Accounts", ["Sam Rivera, @sam", "Marco, @marco", "Mio 澪, @mio"])).toHaveLength(3);
  expect(
    within(dialog)
      .queryAllByRole("status")
      .map((status) => status.textContent),
  ).toContain("3 suggestions");
  expect(text.getAttribute("aria-autocomplete")).toBe("list");
  expect(focusedIn(text)).toBe(true);

  await user.keyboard("{ArrowDown}");
  expect(active(text)).toBe("Marco, @marco");
  expect(listOf(text, "Accounts")?.querySelector("[role=option][aria-selected=true]")?.getAttribute("id")).toBe(
    text.getAttribute("aria-activedescendant"),
  );
  await user.keyboard("{Enter}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(heldBy(text)).toBe("Hi @marco ");
  expect(focusedIn(text)).toBe(true);

  // The caret follows what was written: what comes next goes after its space.
  await pasteInto(user, text, "and hello");
  expect(heldBy(text)).toBe("Hi @marco and hello");
});

test("Escape closes the suggestions and leaves the composer open; a press on an account writes it", async () => {
  const { user, dialog, text } = await composing();
  await user.click(text);
  await pasteInto(user, text, "@m");
  await suggests(text, "Accounts", ["Sam Rivera, @sam", "Marco, @marco", "Mio 澪, @mio"]);

  await user.keyboard("{Escape}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(screen.getByRole("dialog", { name: "New post" }) === dialog).toBe(true);
  expect(focusedIn(text)).toBe(true);
  expect(heldBy(text)).toBe("@m");

  // A new word opens them again; a press picks one and leaves the focus in the text.
  await pasteInto(user, text, " and @ma");
  await suggests(text, "Accounts", ["Marco, @marco"]);
  const option = Array.from(listOf(text, "Accounts")?.querySelectorAll<HTMLElement>("[role=option]") ?? []).find(
    (candidate) => named(candidate) === "Marco, @marco",
  );
  if (option === undefined) throw new Error("test: Marco is not among the accounts suggested");
  await user.click(option);
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(focusedIn(text)).toBe(true);
  expect(heldBy(text)).toBe("@m and @marco ");
});

test("a writer completes a hashtag with Tab, which one undo takes back", async () => {
  const { user, text } = await composing();
  await user.click(text);
  await pasteInto(user, text, "Bread #ba");
  await suggests(text, "Hashtags", [/^#baking, \d+ posts?$/]);

  await user.keyboard("{Tab}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(focusedIn(text)).toBe(true);
  expect(heldBy(text)).toBe("Bread #baking ");

  await user.keyboard("{Control>}z{/Control}");
  expect(heldBy(text)).toBe("Bread #ba");
});

test("a custom emoji is completed by an alias, by pointer and by keyboard, and shows as its image", async () => {
  const { user, text } = await composing();
  await user.click(text);
  await pasteInto(user, text, "Tea :och");

  // ocha is an alias of :teacup:.
  await suggests(text, "Emoji", [":teacup:"]);
  const option = Array.from(listOf(text, "Emoji")?.querySelectorAll<HTMLElement>("[role=option]") ?? []).find(
    (candidate) => named(candidate) === ":teacup:",
  );
  if (option === undefined) throw new Error("test: :teacup: is not among the emoji suggested");
  await user.click(option);
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(focusedIn(text)).toBe(true);
  expect(within(text).getByRole("img", { name: ":teacup:" })).not.toBeNull();
  expect(heldBy(text)).toBe("Tea :teacup: ");

  await pasteInto(user, text, "and :spark");
  await suggests(text, "Emoji", [":sparkle:"]);
  await user.keyboard("{Enter}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(within(text).getByRole("img", { name: ":sparkle:" })).not.toBeNull();
  expect(heldBy(text)).toBe("Tea :teacup: and :sparkle: ");
});
