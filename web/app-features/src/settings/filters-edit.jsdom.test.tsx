// SPDX-License-Identifier: AGPL-3.0-only

// On jsdom: happy-dom gives the focus to any element that is asked to take it, so where a keyword filter's editor
// (#3202) puts the focus once a word is added or removed, and on which field after a Save it did not send or the
// server refused, is decided here (ADR-0171 §4). What the editor sends and says is filters-edit.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import {
  actionChoice,
  addWord,
  alerts,
  endsAt,
  endsChoice,
  filterForm,
  nameField,
  place,
  placeNames,
  removeWord,
  rewrite,
  setEndsAt,
  submitButton,
  wordField,
  words,
  wordsList,
  writes,
  type DrawnFilter,
} from "../test/filter-form";

/** What has the focus, in words: for the error of a test that waits for it. */
const has = () => {
  const element = document.activeElement;
  return `<${element?.tagName.toLowerCase() ?? "nothing"}> "${element?.getAttribute("aria-label") ?? element?.getAttribute("name") ?? element?.textContent ?? ""}"`;
};

/** Waits until `field` has the focus; the page's own description of what has it otherwise. */
const focused = (field: () => HTMLElement | null, name: string) =>
  waitFor(() => {
    if (document.activeElement !== field()) throw new Error(`test: the focus is not on ${name} but on ${has()}`);
  });

/**
 * Whether `field` has the focus once the page is at rest: for a test of a known defect, which reads the page once. The
 * focus is given a frame after what moves it.
 */
async function focusAtRest(field: () => HTMLElement | null): Promise<boolean> {
  await new Promise((resolve) => setTimeout(resolve, 50));
  return document.activeElement === field();
}

/** Whether `field` has the focus once the bar beside the button says something. */
async function focusAfterRefusal(field: () => HTMLElement | null): Promise<boolean> {
  await waitFor(() => {
    if (alerts().length === 0) throw new Error("test: the bar says nothing yet");
  });
  return focusAtRest(field);
}

describe("the words", () => {
  test("Add a word moves the focus to the new word's field", async () => {
    const form = await filterForm({}, { filter: 0 });

    await form.user.click(addWord());

    await focused(() => wordField(3), "Word or phrase 3");
    expect(words()).toHaveLength(3);
  });

  test("Remove of a word moves the focus to the word that takes its place", async () => {
    const form = await filterForm({}, { filter: 1 });

    await form.user.click(removeWord(2));

    await focused(() => wordField(2), "Word or phrase 2");
    expect(wordField(2).value).toBe("polling");
  });

  // As Remove field N of the profile's form: the focus stays among the words while one is left, on the next word, or
  // on the one before when none follows.
  test("Remove of the last of several words moves the focus to the word before it", async () => {
    const form = await filterForm({}, { filter: 0 });

    await form.user.click(removeWord(2));

    await focused(() => wordField(1), "Word or phrase 1");
    expect(words()).toEqual([["finale", true]]);
  });

  test("Remove of the only word moves the focus to Add a word", async () => {
    const form = await filterForm();

    await form.user.click(removeWord(1));

    await focused(addWord, "Add a word");
  });
});

describe("a Save that is not sent moves the focus from the button to the field that is marked", () => {
  /** A new filter with a name, a word and every place: nothing is left out. */
  async function whole() {
    const form = await filterForm();
    await rewrite(form.user, nameField(), "Sourdough");
    await rewrite(form.user, wordField(1), "levain");
    return form;
  }

  test("no name: Name", async () => {
    const form = await whole();
    await rewrite(form.user, nameField(), "");

    await form.user.click(submitButton());

    await focused(nameField, "Name");
    expect(writes(form)).toEqual([]);
  });

  test("a word too long: that word", async () => {
    const form = await whole();
    await form.user.click(addWord());
    await rewrite(form.user, wordField(2), "a".repeat(101));

    await form.user.click(submitButton());

    await focused(() => wordField(2), "Word or phrase 2");
    expect(writes(form)).toEqual([]);
  });

  test("no word, its row left blank: the first word", async () => {
    const form = await whole();
    await rewrite(form.user, wordField(1), "");

    await form.user.click(submitButton());

    await focused(() => wordField(1), "Word or phrase 1");
  });

  test("no word and no row: Add a word", async () => {
    const form = await whole();
    await form.user.click(removeWord(1));

    await form.user.click(submitButton());

    await focused(addWord, "Add a word");
  });

  test("no place: the first place", async () => {
    const form = await whole();
    for (const name of placeNames) await form.user.click(place(name));

    await form.user.click(submitButton());

    await focused(() => place("Home and lists"), "Home and lists");
  });

  test("several things left out: the first of them, Name", async () => {
    const form = await filterForm();
    for (const name of placeNames) await form.user.click(place(name));

    await form.user.click(submitButton());

    await focused(nameField, "Name");
    expect(writes(form)).toEqual([]);
  });

  // The date and time is the field that is marked and described by what is wrong with it, and it takes the focus.
  for (const [what, value] of [
    ["no date and time", ""],
    ["a date and time that is past", "2020-01-01T10:00"],
  ] as const)
    test(`${what}: the date and time`, async () => {
      const form = await whole();
      await form.user.selectOptions(endsChoice(), "At a date and time");
      setEndsAt(value);

      await form.user.click(submitButton());

      expect(await focusAfterRefusal(endsAt)).toBe(true);
    });

  // Everything left out is marked at once, and the focus goes to the first of it in the form's order: the name, the
  // date and time, the words, the places.
  for (const [what, leave, name, field] of [
    [
      "no date and time and a blank word",
      async (form: DrawnFilter) => {
        await form.user.selectOptions(endsChoice(), "At a date and time");
        setEndsAt("");
        await rewrite(form.user, wordField(1), "");
      },
      "Ends at",
      endsAt,
    ],
    [
      "a blank word and no place",
      async (form: DrawnFilter) => {
        await rewrite(form.user, wordField(1), "");
        for (const one of placeNames) await form.user.click(place(one));
      },
      "Word or phrase 1",
      () => wordField(1),
    ],
  ] as const)
    test(`${what}: the first of them, ${name}`, async () => {
      const form = await whole();
      await leave(form);

      await form.user.click(submitButton());

      await focused(field, name);
      expect(writes(form)).toEqual([]);
    });

  // The same helper on a field that does take the focus: what the defect's test above waits for does come.
  test("no name, read as the defect's test reads it: Name", async () => {
    const form = await whole();
    await rewrite(form.user, nameField(), "");

    await form.user.click(submitButton());

    expect(await focusAfterRefusal(nameField)).toBe(true);
  });
});

describe("a Save the server refuses moves the focus to the field its problem names", () => {
  /** A problem of the server about the member `field` of a filter. */
  const refusal = (field: string) =>
    ({ type: "about:blank", title: "This could not be done.", status: 422, field, detail: "Not this." }) as const;

  // The member the problem names, and the field that takes the focus, in the editor of Spoilers. The words as a
  // whole are no one word's: their list takes the focus, which is no control and no stop of Tab.
  for (const [member, name, field] of [
    ["title", "Name", nameField],
    ["action", "Posts that match", actionChoice],
    ["keywords[1].keyword", "Word or phrase 2", () => wordField(2)],
    ["keywords", "the list Words", wordsList],
    ["contexts", "Home and lists", () => place("Home and lists")],
    ["expires_at", "Ends", endsChoice],
  ] as const)
    test(`${member}: ${name}`, async () => {
      const form = await filterForm({}, { filter: 0 });
      form.fake.script("PATCH /filters/{id}", { times: 1, fail: refusal(member) });

      await form.user.click(submitButton());

      await focused(field, name);
      expect(form.told.includes("back")).toBe(false);
    });

  test("more words than a filter or all the filters can have: the list Words", async () => {
    const form = await filterForm({}, { filter: 0 });
    form.fake.script("PATCH /filters/{id}", {
      times: 1,
      fail: {
        type: "/api/v1/problems/too-many-filter-keywords",
        title: "You have as many words in your filters as you may",
        status: 422,
      },
    });

    await form.user.click(submitButton());

    await focused(wordsList, "the list Words");
    // From the list, Tab goes on into its first word; and back from the word, Tab passes the list.
    await form.user.tab();
    await focused(() => wordField(1), "Word or phrase 1");
    await form.user.tab({ shift: true });
    expect(document.activeElement === wordsList()).toBe(false);
  });

  test("a refusal that names no field leaves the focus on the button", async () => {
    const form = await filterForm({}, { filter: 0 });
    form.fake.script("PATCH /filters/{id}", { times: 1, fail: 500 });

    await form.user.click(submitButton());

    await waitFor(() => {
      if (alerts().length === 0) throw new Error("test: the bar says nothing yet");
    });
    expect(`${has()} ${String(document.activeElement === screen.getByRole("button", { name: "Save" }))}`).toBe(
      `<button> "Save" true`,
    );
  });
});
