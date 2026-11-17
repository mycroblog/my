// SPDX-License-Identifier: AGPL-3.0-only

// The listbox of the suggestions for the word at the caret. The text that owns it keeps the focus and names the
// active option with aria-activedescendant, by the ids this gives its options.

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SuggestionList, type Suggestion, type SuggestionListProps } from "./suggestions";

const items: Suggestion[] = [
  { kind: "account", name: "Sam", handle: "@sam", initialOf: "Sam", label: "Sam, @sam" },
  { kind: "account", name: "Mika", handle: "@mika@social.example", initialOf: "Mika" },
  { kind: "hashtag", name: "bread", uses: "12 posts" },
  { kind: "emoji", name: ":party:", image: "https://m.example/party.webp" },
  { kind: "emoji", name: "sparkles", text: "✨" },
];

/** The suggestions under a text that has the focus, as the composer shows them, and the indexes chosen, in order. */
function suggestions(props: Partial<SuggestionListProps> = {}) {
  const chosen: number[] = [];
  render(
    <>
      <textarea aria-label="Text" />
      <SuggestionList
        id="suggest"
        label="Suggestions"
        items={items}
        active={0}
        onChoose={(index) => chosen.push(index)}
        {...props}
      />
    </>,
  );
  screen.getByRole("textbox", { name: "Text" }).focus();
  return chosen;
}

test("the suggestions are a listbox named by its label, with the id the text controls it by", () => {
  suggestions();

  const list = screen.getByRole("listbox", { name: "Suggestions" });
  expect(list.id).toBe("suggest");
  expect(within(list).getAllByRole("option")).toHaveLength(5);
});

test("each option has the id of its place, and the active one alone is selected", () => {
  suggestions({ active: 2 });

  const options = screen.getAllByRole("option");
  expect(options.map((option) => option.id)).toEqual(["suggest-0", "suggest-1", "suggest-2", "suggest-3", "suggest-4"]);
  expect(screen.getAllByRole("option", { selected: true }).map((option) => option.id)).toEqual(["suggest-2"]);
  expect(screen.getAllByRole("option", { selected: false })).toHaveLength(4);
});

test("an option reads as its label where it has one, else as its words; a glyph or an image adds nothing", () => {
  suggestions();

  const names = ["Sam, @sam", /^Mika\s*@mika\s*@social\.example$/, /^#bread\s*12 posts$/, ":party:", "sparkles"];
  expect(names.map((name) => screen.getByRole("option", { name }).id)).toEqual([
    "suggest-0",
    "suggest-1",
    "suggest-2",
    "suggest-3",
    "suggest-4",
  ]);
});

test("a press on an option chooses it by its place, and the text keeps the focus", async () => {
  const user = userEvent.setup();
  const chosen = suggestions();

  await user.click(screen.getByRole("option", { name: ":party:" }));
  await user.click(screen.getByRole("option", { name: "Sam, @sam" }));

  expect(chosen).toEqual([3, 0]);
  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Text" }));
});

test("no option takes the focus from the keyboard: Tab passes the list", async () => {
  const user = userEvent.setup();
  suggestions();

  await user.tab();

  expect(screen.getByRole("listbox", { name: "Suggestions" }).contains(document.activeElement)).toBe(false);
});

for (const kind of ["loading", "empty"] as const)
  test(`while the suggestions are ${kind}, the line that says so stands in the list's place`, () => {
    suggestions({ items: [], status: { kind, text: kind === "loading" ? "Looking…" : "No one found" } });

    expect(screen.getByText(kind === "loading" ? "Looking…" : "No one found")).not.toBeNull();
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.queryByRole("option")).toBeNull();
  });
