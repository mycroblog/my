// SPDX-License-Identifier: AGPL-3.0-only

// A keyword filter's editor, the screen under the settings' Keyword filters (#3202, FR-209, ADR-0157 §6, §9): it opens
// empty for a new filter and with what the server holds for one of the reader's; its words are a list added to and
// removed from, within the most a filter and all the filters can have; Add or Save sends the whole filter as the API
// takes it, with the end the reader chose, and tells its owner to go back; what the reader left out and what the
// server refuses shows under the field it is about, with the bar beside the button saying that something is marked;
// Delete asks first. The editor tells its owner whether it holds unsaved changes, and asks before they are left when
// its owner says the reader is leaving. Where the focus goes is filters-edit.jsdom.test.tsx; the list of filters is
// filters.dom.test.tsx; the way to the editor, the way back and the held navigation are the app's
// (web/app/src/settings-filters.dom.test.tsx); the fields by themselves are the design system's.

import { describe, expect, test } from "bun:test";
import { keywordFiltersQuery, type KeywordFilterRequest } from "@my/app-data";
import { screen, waitFor, within } from "@testing-library/react";
import {
  actionChoice,
  addWord,
  alerts,
  chosen,
  describedBy,
  endsAt,
  endsChoice,
  filterForm,
  invalid,
  nameField,
  offered,
  place,
  placeNames,
  removeWord,
  rewrite,
  said,
  section,
  setEndsAt,
  submitButton,
  wholeWord,
  wordField,
  words,
  wordsList,
  writes,
  type DrawnFilter,
} from "../test/filter-form";
import { plain } from "../test/relationships";

const minute = 60 * 1000;
const hour = 60 * minute;
const marked = "To save, fix what is marked.";
const nameHint = "A post it folds shows this name.";
const ends = [
  "Never",
  "In 30 minutes",
  "In 1 hour",
  "In 6 hours",
  "In 12 hours",
  "In 1 day",
  "In 1 week",
  "At a date and time",
];

/** The headings of the screen: its title, then its sections. */
const headings = () => screen.getAllByRole("heading").map((heading) => plain(heading.textContent));
/** A moment as the editor writes it: the day and the time. */
const moment = (at: string | undefined) =>
  plain(new Intl.DateTimeFormat("en", { dateStyle: "medium", timeStyle: "short" }).format(new Date(at ?? "")));
/** The lines under the heading of Words, after the one that says what the words do. */
const room = () =>
  section("Words")
    .queryAllByText(/^(You can add|This filter has 100 words|Your filters have)/)
    .map((line) => line.textContent);

/** The reader's filter at `index` as the server holds it, as a request that would save it unchanged. */
async function held({ data }: DrawnFilter, index: number): Promise<KeywordFilterRequest> {
  const { items } = await data.queryClient.query({ ...keywordFiltersQuery(data), staleTime: Infinity });
  const filter = items[index];
  if (filter === undefined) throw new Error(`test: the reader has no filter ${String(index)}`);
  const { title, action, contexts, keywords, expires_at } = filter;
  return { title, action, contexts, keywords, ...(expires_at === undefined ? {} : { expires_at }) };
}

/** Presses Add or Save, and returns once the editor told its owner to go back: the filter is saved. */
async function saved(form: DrawnFilter): Promise<void> {
  await form.user.click(submitButton());
  await waitFor(() => {
    if (!form.told.includes("back")) throw new Error("test: the editor did not go back yet");
  });
}

/** Presses Add or Save, and returns once the bar beside it says `words`. */
async function refused(form: DrawnFilter, words: string | RegExp = marked): Promise<void> {
  await form.user.click(submitButton());
  await waitFor(() => {
    const [alert, ...others] = alerts();
    if (alert === undefined || others.length > 0 || (typeof words === "string" ? alert !== words : !words.test(alert)))
      throw new Error(`test: the bar says ${JSON.stringify(alerts())}`);
  });
}

/** A filter that folds at home, with `count` words. */
const filterOf = (title: string, count: number) => ({
  title,
  action: "warn" as const,
  contexts: ["home" as const],
  keywords: Array.from({ length: count }, (_, i) => ({ keyword: `${title} ${String(i + 1)}`, whole_word: false })),
});

/** A problem of the server about the member `field` of a filter, in its own words. */
const refusal = (field: string, detail: string) =>
  ({ type: "about:blank", title: "This could not be done.", status: 422, field, detail }) as const;

describe("the editor opens", () => {
  test("for a new filter: no name, folding, never ending, one empty word, every place, and Add", async () => {
    const form = await filterForm();

    expect(headings()).toEqual(["New filter", "Name and action", "Words 1", "Where it applies"]);
    expect(nameField().value).toBe("");
    expect(describedBy(nameField())).toEqual([nameHint]);
    expect(offered(actionChoice())).toEqual(["Fold behind the filter's name", "Hide completely"]);
    expect(chosen(actionChoice())).toBe("Fold behind the filter's name");
    expect(offered(endsChoice())).toEqual(ends);
    expect(chosen(endsChoice())).toBe("Never");
    expect(endsAt()).toBeNull();
    expect(words()).toEqual([["", false]]);
    expect(placeNames.map((name) => [name, place(name).checked])).toEqual(placeNames.map((name) => [name, true]));
    expect(submitButton().textContent).toBe("Add");
    // A filter not made yet has nothing to delete.
    expect(screen.queryAllByRole("button", { name: "Delete…" })).toHaveLength(0);
    // Nothing is wrong, nothing is unsaved, and nothing was sent.
    expect(invalid()).toEqual([]);
    expect(alerts()).toEqual([]);
    expect(said()).toEqual([]);
    expect(form.told.includes("unsaved:true")).toBe(false);
    expect(writes(form)).toEqual([]);
  });

  test("for one of the reader's filters: its name, what it does, its words, its places, Save, and the way to delete it", async () => {
    const form = await filterForm({}, { filter: 0 });

    expect(headings()).toEqual(["Edit filter", "Name and action", "Words 2", "Where it applies", "Delete filter"]);
    expect(nameField().value).toBe("Spoilers");
    expect(chosen(actionChoice())).toBe("Fold behind the filter's name");
    // A filter that never ends has nothing to keep: the same choices as a new one.
    expect(offered(endsChoice())).toEqual(ends);
    expect(chosen(endsChoice())).toBe("Never");
    expect(words()).toEqual([
      ["finale", true],
      ["season 3", false],
    ]);
    expect(placeNames.filter((name) => place(name).checked)).toEqual(["Home and lists", "Public timelines", "Threads"]);
    expect(submitButton().textContent).toBe("Save");
    expect(
      section("Delete filter").getAllByText(
        "Its words are deleted with it, and the posts it folded or hid show again.",
      ),
    ).toHaveLength(1);
    expect(section("Delete filter").getAllByRole("button", { name: "Delete…" })).toHaveLength(1);
    expect(invalid()).toEqual([]);
    expect(alerts()).toEqual([]);
    expect(form.requests).toContain(`GET /filters/${form.id ?? ""}`);
  });

  test("for a filter that hides and ends: Hide completely, and its end kept unless another is chosen", async () => {
    const form = await filterForm({}, { filter: 1 });
    const { expires_at } = await held(form, 1);

    expect(nameField().value).toBe("Election season");
    expect(chosen(actionChoice())).toBe("Hide completely");
    expect(offered(endsChoice()).map(plain)).toEqual([`No change (ends ${moment(expires_at)})`, ...ends]);
    expect(plain(chosen(endsChoice()))).toBe(`No change (ends ${moment(expires_at)})`);
    expect(describedBy(endsChoice())).toEqual([]);
    expect(endsAt()).toBeNull();
  });

  test("for a filter that ended: its past end kept, and a note under Ends that says it applies nowhere, without calling the choice wrong", async () => {
    const form = await filterForm({}, { filter: 2 });
    const { expires_at } = await held(form, 2);

    expect(plain(chosen(endsChoice()))).toBe(`No change (ended ${moment(expires_at)})`);
    expect(offered(endsChoice()).slice(1)).toEqual(ends);
    expect(describedBy(endsChoice()).map(plain)).toEqual([
      `This filter ended on ${moment(expires_at)} and applies nowhere. Choose a new end to turn it on again.`,
    ]);
    expect(invalid()).toEqual([]);
    expect(alerts()).toEqual([]);
  });

  test("a date and time to end at is asked for only once Ends says so, and starts later than now", async () => {
    const form = await filterForm();
    expect(endsAt()).toBeNull();

    await form.user.selectOptions(endsChoice(), "At a date and time");

    const field = endsAt();
    expect(field?.type).toBe("datetime-local");
    // A browser gives the value in the form YYYY-MM-DDTHH:mm, which sorts as time does.
    const now = new Date(Date.now() - new Date().getTimezoneOffset() * minute).toISOString().slice(0, 16);
    expect((field?.value ?? "") > now).toBe(true);
    expect(invalid()).toEqual([]);

    await form.user.selectOptions(endsChoice(), "In 1 day");

    expect(endsAt()).toBeNull();
  });

  test("in Japanese its sections, its fields and its buttons read in it", async () => {
    await filterForm({}, { filter: 0, locale: "ja" });

    expect(headings()).toEqual(["フィルタを編集", "名前と動作", "言葉 2", "適用する場所", "フィルタを削除"]);
    expect(screen.getAllByRole("textbox", { name: "名前" })).toHaveLength(1);
    expect(screen.getAllByRole("combobox", { name: "一致した投稿" })).toHaveLength(1);
    expect(screen.getAllByRole("combobox", { name: "期限" })).toHaveLength(1);
    expect(screen.getAllByRole("textbox", { name: "言葉・フレーズ 2" })).toHaveLength(1);
    expect(screen.getAllByRole("checkbox", { name: "単語全体" })).toHaveLength(2);
    expect(screen.getAllByRole("checkbox", { name: "ホームとリスト" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "言葉を追加" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "削除…" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "保存" })).toHaveLength(1);
  });

  test("the way back, named after the entry, is the owner's to follow", async () => {
    const form = await filterForm({}, { filter: 0 });

    await form.user.click(screen.getByRole("button", { name: "Keyword filters" }));

    expect(form.told).toContain("back");
  });

  test("at an address that names no filter of the reader's: that the filter does not exist, and the way to all filters, with no field", async () => {
    await filterForm({}, { filter: "nothing" });

    const missing = await screen.findByRole("alert");
    expect(plain(missing.textContent)).toMatch(
      /^This filter does not exist\. It may have been deleted\.\s*See all filters$/,
    );
    expect(within(missing).getByRole("link", { name: "See all filters" }).getAttribute("href")).toBe(
      "/settings/filters",
    );
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(screen.queryAllByRole("button", { name: /^(Add|Save|Delete…)$/ })).toHaveLength(0);
  });
});

describe("the words", () => {
  test("each is a row of the list Words: a field named by its number with no label drawn beside it, Whole word, and Remove named by the number", async () => {
    await filterForm({}, { filter: 0 });

    const rows = within(section("Words").getByRole("list", { name: "Words" })).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    for (const [i, row] of rows.entries()) {
      const n = String(i + 1);
      expect(within(row).getAllByRole("textbox", { name: `Word or phrase ${n}` })).toHaveLength(1);
      expect(within(row).getAllByRole("checkbox", { name: "Whole word" })).toHaveLength(1);
      expect(within(row).getAllByRole("button", { name: `Remove word ${n}` })).toHaveLength(1);
    }
    expect(
      section("Words").getAllByText(
        "A post that contains any of them matches. With “Whole word”, a word does not match inside a longer one.",
      ),
    ).toHaveLength(1);
  });

  test("Add a word adds an empty row after the last, and the heading counts the rows", async () => {
    const form = await filterForm({}, { filter: 0 });

    await form.user.click(addWord());

    expect(words()).toEqual([
      ["finale", true],
      ["season 3", false],
      ["", false],
    ]);
    expect(headings()[2]).toBe("Words 3");
    expect(removeWord(3)).not.toBeNull();
    expect(writes(form)).toEqual([]);
  });

  test("Remove takes that word away, and the words after it move up", async () => {
    const form = await filterForm({}, { filter: 1 });
    expect(words().map(([word]) => word)).toEqual(["election", "ballot", "polling"]);

    await form.user.click(removeWord(2));

    expect(words()).toEqual([
      ["election", true],
      ["polling", false],
    ]);
    expect(headings()[2]).toBe("Words 2");
    expect(screen.queryAllByRole("button", { name: "Remove word 3" })).toHaveLength(0);
    expect(writes(form)).toEqual([]);
  });

  test("with its last word removed, where the list would be says that the filter has no words, and Add a word brings a row back", async () => {
    const form = await filterForm();
    const none = "This filter has no words.";
    expect(section("Words").queryAllByText(none)).toHaveLength(0);

    await form.user.click(removeWord(1));

    expect(section("Words").getAllByText(none)).toHaveLength(1);
    expect(section("Words").queryAllByRole("list")).toHaveLength(0);
    expect(headings()[2]).toBe("Words");

    await form.user.click(addWord());

    expect(words()).toEqual([["", false]]);
    expect(section("Words").queryAllByText(none)).toHaveLength(0);
  });
});

// ADR-0157 §9: 100 words a filter, 1,000 over all the reader's filters.
describe("the room left for words", () => {
  const full = "This filter has 100 words, the most a filter can have. Remove one to add another.";
  const allFull =
    "Your filters have 1,000 words in all, the most they can have. Remove some here or from another filter to add more.";

  test("is not said while more than ten can be added, then counted down to one", async () => {
    const form = await filterForm({ settings: { filters: [filterOf("Some", 89)] } }, { filter: 0 });
    expect(room()).toEqual([]);

    await form.user.click(addWord());
    expect(room()).toEqual(["You can add 10 more words."]);

    for (let n = 91; n <= 99; n += 1) await form.user.click(addWord());
    expect(words()).toHaveLength(99);
    expect(room()).toEqual(["You can add one more word."]);
    expect(addWord().disabled).toBe(false);
  });

  test("at 100 words in the filter, Add a word is disabled and described by the line that says so, until a word is removed", async () => {
    const form = await filterForm({ settings: { filters: [filterOf("Full", 100)] } }, { filter: 0 });

    expect(headings()[2]).toBe("Words 100");
    expect(room()).toEqual([full]);
    expect(addWord().disabled).toBe(true);
    expect(describedBy(addWord())).toEqual([full]);

    await form.user.click(removeWord(100));

    expect(room()).toEqual(["You can add one more word."]);
    expect(addWord().disabled).toBe(false);
    expect(describedBy(addWord())).not.toContain(full);
  });

  test("counts the words of the reader's other filters too: at 1,000 in all, Add a word is disabled and described by the line that says so", async () => {
    // 993 words in ten other filters: a new filter, with its one row, leaves room for six more.
    const others = [
      ...Array.from({ length: 9 }, (_, i) => filterOf(`Topic ${String(i + 1)}`, 100)),
      filterOf("Last", 93),
    ];
    const form = await filterForm({ settings: { filters: others } });
    expect(room()).toEqual(["You can add 6 more words."]);

    for (let n = 0; n < 6; n += 1) await form.user.click(addWord());

    expect(words()).toHaveLength(7);
    expect(room()).toEqual([allFull]);
    expect(addWord().disabled).toBe(true);
    expect(describedBy(addWord())).toEqual([allFull]);
  });

  test("a filter's own words count once: one of 100 words among 1,000 in all says the filter's line, not twice its words", async () => {
    const others = [
      ...Array.from({ length: 9 }, (_, i) => filterOf(`Topic ${String(i + 1)}`, 100)),
      filterOf("Mine", 90),
    ];
    await filterForm({ settings: { filters: others } }, { filter: 9 });

    // 900 in the others and 90 here: ten more fit.
    expect(room()).toEqual(["You can add 10 more words."]);
    expect(addWord().disabled).toBe(false);
  });
});

describe("Add, for a new filter", () => {
  test("sends one POST with the filter as written, and tells the owner to go back with nothing unsaved", async () => {
    const form = await filterForm();
    const sent = form.fake.script("POST /filters");
    await rewrite(form.user, nameField(), "Sourdough");
    await rewrite(form.user, wordField(1), "levain");
    await form.user.click(wholeWord(1));
    await form.user.click(addWord());
    await rewrite(form.user, wordField(2), "starter culture");
    await form.user.click(place("Profiles"));
    await form.user.selectOptions(actionChoice(), "Hide completely");

    await saved(form);

    expect(writes(form)).toEqual(["POST /filters"]);
    const [body, ...others] = sent.seen.map((request) => request.body);
    expect(others).toEqual([]);
    expect(body).toEqual({
      title: "Sourdough",
      action: "hide",
      contexts: ["home", "notifications", "public", "thread"],
      keywords: [
        { keyword: "levain", whole_word: true },
        { keyword: "starter culture", whole_word: false },
      ],
    });
    // A filter that never ends names no end.
    expect(Object.keys(body ?? {})).not.toContain("expires_at");
    expect(form.told.slice(-2)).toEqual(["unsaved:false", "back"]);
    expect(alerts()).toEqual([]);
  });

  test("the server holds the filter once the owner is told", async () => {
    const form = await filterForm();
    await rewrite(form.user, nameField(), "Sourdough");
    await rewrite(form.user, wordField(1), "levain");

    await saved(form);

    const { items } = await form.data.queryClient.query({ ...keywordFiltersQuery(form.data), staleTime: 0 });
    expect(items.map((filter) => filter.title)).toEqual(["Spoilers", "Election season", "Marathon week", "Sourdough"]);
    expect(items.at(-1)).toMatchObject({
      action: "warn",
      contexts: ["home", "notifications", "public", "thread", "account"],
    });
  });
});

describe("Save, for one of the reader's filters", () => {
  /** A change of a filter a reader makes in the editor: the filter, what they do, and what Save sends in place of the filter. */
  type Change = {
    what: string;
    filter: number;
    change: (form: DrawnFilter) => Promise<void>;
    sends: (as: KeywordFilterRequest) => KeywordFilterRequest;
  };
  const without = ({ expires_at: _end, ...rest }: KeywordFilterRequest): KeywordFilterRequest => rest;

  /** Writes another name: a change that leaves everything else of the filter as it is. */
  const renamed = (form: DrawnFilter) => rewrite(form.user, nameField(), "Renamed");

  const changes: readonly Change[] = [
    {
      what: "another name of a filter that ends: its end as the server holds it",
      filter: 1,
      change: renamed,
      sends: (as) => ({ ...as, title: "Renamed" }),
    },
    {
      what: "another name of a filter that ended: its past end, so that it stays expired",
      filter: 2,
      change: renamed,
      sends: (as) => ({ ...as, title: "Renamed" }),
    },
    {
      what: "Never, for a filter that ends: no end",
      filter: 1,
      change: async (form) => {
        await form.user.selectOptions(endsChoice(), "Never");
      },
      sends: without,
    },
    {
      what: "another name",
      filter: 0,
      change: (form) => rewrite(form.user, nameField(), "Spoilers and more"),
      sends: (as) => ({ ...as, title: "Spoilers and more" }),
    },
    {
      what: "hiding in place of folding",
      filter: 0,
      change: async (form) => {
        await form.user.selectOptions(actionChoice(), "Hide completely");
      },
      sends: (as) => ({ ...as, action: "hide" }),
    },
    {
      what: "a place more and a place less: the places in the editor's order",
      filter: 0,
      change: async (form) => {
        await form.user.click(place("Profiles"));
        await form.user.click(place("Notifications"));
        await form.user.click(place("Threads"));
      },
      sends: (as) => ({ ...as, contexts: ["home", "notifications", "public", "account"] }),
    },
    {
      what: "a word rewritten, with the spaces around it left out",
      filter: 0,
      change: (form) => rewrite(form.user, wordField(1), "  series finale  "),
      sends: (as) => ({ ...as, keywords: [{ keyword: "series finale", whole_word: true }, ...as.keywords.slice(1)] }),
    },
    {
      what: "Whole word turned on and off",
      filter: 0,
      change: async (form) => {
        await form.user.click(wholeWord(1));
        await form.user.click(wholeWord(2));
      },
      sends: (as) => ({
        ...as,
        keywords: [
          { keyword: "finale", whole_word: false },
          { keyword: "season 3", whole_word: true },
        ],
      }),
    },
    {
      what: "a word removed and one added: the words in the editor's order",
      filter: 0,
      change: async (form) => {
        await form.user.click(removeWord(1));
        await form.user.click(addWord());
        await rewrite(form.user, wordField(2), "season 4");
        await form.user.click(wholeWord(2));
      },
      sends: (as) => ({
        ...as,
        keywords: [
          { keyword: "season 3", whole_word: false },
          { keyword: "season 4", whole_word: true },
        ],
      }),
    },
    {
      what: "a row added and left blank: a blank row is no word",
      filter: 0,
      change: async (form) => {
        await form.user.click(addWord());
        await form.user.click(addWord());
        await rewrite(form.user, wordField(4), "season 4");
      },
      sends: (as) => ({ ...as, keywords: [...as.keywords, { keyword: "season 4", whole_word: false }] }),
    },
  ];

  for (const { what, filter, change, sends } of changes)
    test(`sends one PATCH of the filter, whole: ${what}`, async () => {
      const form = await filterForm({}, { filter });
      const as = await held(form, filter);
      const sent = form.fake.script("PATCH /filters/{id}");
      await change(form);

      await saved(form);

      expect(writes(form)).toEqual([`PATCH /filters/${form.id ?? ""}`]);
      expect(sent.seen.map((request) => request.body)).toEqual([sends(as)]);
      expect(Object.keys(sent.seen[0]?.body ?? {}).includes("expires_at")).toBe("expires_at" in sends(as));
      expect(form.told.slice(-2)).toEqual(["unsaved:false", "back"]);
    });

  for (const filter of [0, 1, 2])
    test(`nothing changed of filter ${String(filter)}: Save sends nothing and goes back`, async () => {
      const form = await filterForm({}, { filter });
      const sent = form.fake.script("PATCH /filters/{id}");

      await saved(form);

      expect(writes(form)).toEqual([]);
      expect(sent.seen).toHaveLength(0);
      expect(form.told.includes("unsaved:true")).toBe(false);
    });

  // What Ends offers, and how long from now the filter then ends.
  for (const [choice, after] of [
    ["In 30 minutes", 30 * minute],
    ["In 1 hour", hour],
    ["In 6 hours", 6 * hour],
    ["In 12 hours", 12 * hour],
    ["In 1 day", 24 * hour],
    ["In 1 week", 7 * 24 * hour],
  ] as const)
    test(`${choice} sends the moment that long after the press as the filter's end`, async () => {
      const form = await filterForm({}, { filter: 2 });
      const sent = form.fake.script("PATCH /filters/{id}");
      await form.user.selectOptions(endsChoice(), choice);
      const before = Date.now();

      await saved(form);

      const end = Date.parse(sent.seen[0]?.body.expires_at ?? "");
      expect(end).toBeGreaterThanOrEqual(before + after);
      expect(end).toBeLessThanOrEqual(Date.now() + after);
    });

  // Whose clock the date and time is read on is the app's to say, which knows the reader's time zone
  // (web/app/src/settings-filters.dom.test.tsx); drawn alone, the editor reads it on the device's.
  test("At a date and time sends the moment written as the filter's end", async () => {
    const form = await filterForm({}, { filter: 0 });
    const sent = form.fake.script("PATCH /filters/{id}");
    await form.user.selectOptions(endsChoice(), "At a date and time");

    setEndsAt("2099-02-03T04:05");
    await saved(form);

    expect(sent.seen.map((request) => request.body.expires_at)).toEqual([new Date("2099-02-03T04:05").toISOString()]);
  });

  test("while the server has not answered, Save is busy and nothing of the editor can be changed", async () => {
    const form = await filterForm({ settings: { filterSave: "waits" } }, { filter: 1 });
    await form.user.selectOptions(endsChoice(), "At a date and time");

    await form.user.click(submitButton());

    await waitFor(() => {
      expect(submitButton().getAttribute("aria-busy")).toBe("true");
    });
    // A text field takes no change and keeps the focus it may have: read-only.
    expect([nameField(), wordField(1), wordField(2), wordField(3)].map((field) => field.readOnly)).toEqual([
      true,
      true,
      true,
      true,
    ]);
    const fixed: (HTMLInputElement | HTMLSelectElement | HTMLButtonElement | null)[] = [
      actionChoice(),
      endsChoice(),
      endsAt(),
      ...[1, 2, 3].map(wholeWord),
      ...placeNames.map(place),
      addWord(),
      ...[1, 2, 3].map(removeWord),
      screen.getByRole<HTMLButtonElement>("button", { name: "Delete…" }),
    ];
    expect(fixed.map((control) => control?.disabled)).toEqual(fixed.map(() => true));
    // The way back stays.
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Keyword filters" }).disabled).toBe(false);
    // A second press sends no second change.
    const sent = form.fake.script("PATCH /filters/{id}");
    await form.user.click(submitButton());
    expect(sent.seen).toHaveLength(0);
    expect(form.told.includes("back")).toBe(false);
  });
});

describe("what the reader left out is said before anything is sent", () => {
  /** A new filter with a name, a word and every place: nothing is left out. */
  async function whole() {
    const form = await filterForm();
    await rewrite(form.user, nameField(), "Sourdough");
    await rewrite(form.user, wordField(1), "levain");
    return form;
  }

  test("no name: the sentence under Name, which is marked, and the bar says that something is marked; a name written takes the mark away", async () => {
    const form = await whole();
    await rewrite(form.user, nameField(), "");

    await refused(form);

    expect(invalid()).toEqual(["Name"]);
    expect(describedBy(nameField())).toEqual([nameHint, "Give the filter a name."]);
    expect(said()).toEqual([marked]);
    expect(writes(form)).toEqual([]);
    expect(form.told.includes("back")).toBe(false);
    // What was written stays.
    expect(words()).toEqual([["levain", false]]);

    await rewrite(form.user, nameField(), "Sourdough");

    expect(invalid()).toEqual([]);
    expect(describedBy(nameField())).toEqual([nameHint]);
    expect(alerts()).toEqual([]);
    await saved(form);
    expect(writes(form)).toEqual(["POST /filters"]);
  });

  test("a name of spaces alone is no name", async () => {
    const form = await whole();
    await rewrite(form.user, nameField(), "   ");

    await refused(form);

    expect(invalid()).toEqual(["Name"]);
    expect(describedBy(nameField())).toContain("Give the filter a name.");
    expect(writes(form)).toEqual([]);
  });

  test("a name of 51 characters: that it is too long, under Name; of 50, each emoji one character, it is sent", async () => {
    const form = await whole();
    const sent = form.fake.script("POST /filters");
    await rewrite(form.user, nameField(), "😀".repeat(51));

    await refused(form);

    expect(invalid()).toEqual(["Name"]);
    expect(describedBy(nameField())).toEqual([nameHint, "The name must be at most 50 characters."]);
    expect(writes(form)).toEqual([]);

    await rewrite(form.user, nameField(), "😀".repeat(50));
    await saved(form);

    expect(sent.seen.map((request) => request.body.title)).toEqual(["😀".repeat(50)]);
  });

  // What is marked is where a word goes: the blank row's field, or with no row the button that adds one, which holds
  // no value to call wrong.
  for (const [rows, how, where] of [
    [1, "its one row left blank", "its field, which is marked"],
    [0, "no row at all", "Add a word, and nothing is marked as a wrong value"],
  ] as const)
    test(`no word, with ${how}: the sentence under the words describes ${where}, and nothing is sent; a word written is taken`, async () => {
      const form = await whole();
      const missing = "Add at least one word or phrase.";
      if (rows === 0) await form.user.click(removeWord(1));
      else await rewrite(form.user, wordField(1), "  ");

      await refused(form);

      expect(section("Words").getAllByText(missing)).toHaveLength(1);
      expect(invalid()).toEqual(rows === 0 ? [] : ["Word or phrase 1"]);
      expect(describedBy(rows === 0 ? addWord() : wordField(1))).toEqual([missing]);
      expect(addWord().getAttribute("aria-invalid")).toBeNull();
      expect(said()).toEqual([marked]);
      expect(writes(form)).toEqual([]);
      expect(nameField().value).toBe("Sourdough");

      if (rows === 0) await form.user.click(addWord());
      await rewrite(form.user, wordField(1), "levain");

      expect(section("Words").queryAllByText(missing)).toHaveLength(0);
      expect(invalid()).toEqual([]);
      expect([describedBy(wordField(1)), describedBy(addWord())]).toEqual([[], []]);
      await saved(form);
      expect(writes(form)).toEqual(["POST /filters"]);
    });

  test("everything left out is marked at once, with the focus on the first of it; each mark goes as its value is put right, and what the bar says with the last", async () => {
    const form = await filterForm();
    const noPlace = "Choose at least one place.";
    /** What describes each place. */
    const places = () => placeNames.map((name) => describedBy(place(name)));
    await form.user.selectOptions(endsChoice(), "At a date and time");
    setEndsAt("");
    for (const name of placeNames) await form.user.click(place(name));

    await refused(form);

    // No name, no date and time, a blank word and no place, in the form's order.
    expect(invalid()).toEqual(["Name", "Ends at", "Word or phrase 1"]);
    expect(places()).toEqual(placeNames.map(() => [noPlace]));
    expect(document.activeElement === nameField()).toBe(true);
    expect(writes(form)).toEqual([]);

    // The editor checks itself again at each change, while its own check stands.
    await rewrite(form.user, nameField(), "Sourdough");
    expect(invalid()).toEqual(["Ends at", "Word or phrase 1"]);
    expect(describedBy(nameField())).toEqual([nameHint]);
    setEndsAt("2099-02-03T04:05");
    expect(invalid()).toEqual(["Word or phrase 1"]);
    await rewrite(form.user, wordField(1), "levain");
    expect(invalid()).toEqual([]);
    expect(places()).toEqual(placeNames.map(() => [noPlace]));
    expect(alerts()).toEqual([marked]);

    await form.user.click(place("Threads"));

    expect(places()).toEqual(placeNames.map(() => []));
    expect(alerts()).toEqual([]);
    expect(writes(form)).toEqual([]);
  });

  test("a word of 101 characters: that it is too long, under that word alone; of 100, it is sent", async () => {
    const form = await whole();
    const sent = form.fake.script("POST /filters");
    await form.user.click(addWord());
    await rewrite(form.user, wordField(2), "a".repeat(101));

    await refused(form);

    expect(invalid()).toEqual(["Word or phrase 2"]);
    expect(describedBy(wordField(2))).toEqual(["A word must be at most 100 characters."]);
    expect(describedBy(wordField(1))).toEqual([]);
    expect(writes(form)).toEqual([]);

    await rewrite(form.user, wordField(2), "a".repeat(100));
    await saved(form);

    expect(sent.seen.map((request) => request.body.keywords.map((word) => word.keyword.length))).toEqual([[6, 100]]);
  });

  test("no place: the sentence describes every place, and a place checked takes it away", async () => {
    const form = await whole();
    const missing = "Choose at least one place.";
    for (const name of placeNames) await form.user.click(place(name));

    await refused(form);

    expect(placeNames.map((name) => describedBy(place(name)))).toEqual(placeNames.map(() => [missing]));
    expect(section("Where it applies").getAllByText(missing)).toHaveLength(1);
    expect(writes(form)).toEqual([]);

    await form.user.click(place("Threads"));

    expect(placeNames.map((name) => describedBy(place(name)))).toEqual(placeNames.map(() => []));
    expect(alerts()).toEqual([]);
    const sent = form.fake.script("POST /filters");
    await saved(form);
    expect(sent.seen.map((request) => request.body.contexts)).toEqual([["thread"]]);
  });

  for (const [what, value, sentence] of [
    ["no date and time", "", "Choose a date and time."],
    ["a date and time that is past", "2020-01-01T10:00", "Choose a date and time later than now."],
  ] as const)
    test(`${what}, where the filter ends at one: the sentence under the date and time, which is marked`, async () => {
      const form = await whole();
      await form.user.selectOptions(endsChoice(), "At a date and time");
      setEndsAt(value);

      await refused(form);

      expect(invalid()).toEqual(["Ends at"]);
      expect(describedBy(endsAt() ?? document.body)).toEqual([sentence]);
      expect(writes(form)).toEqual([]);

      setEndsAt("2099-02-03T04:05");
      const sent = form.fake.script("POST /filters");
      await saved(form);
      expect(sent.seen.map((request) => request.body.expires_at?.slice(0, 4))).toEqual(["2099"]);
    });
});

describe("what the server refuses shows where it belongs", () => {
  const nameRule = "The name must be 1 to 50 characters on one line, without tabs or other control characters.";
  const wordRule = "A word must be 1 to 100 characters, without tabs or other control characters.";

  /** How a script of the fake fails a request: a status, a whole problem, or no answer. */
  type Failure = NonNullable<NonNullable<Parameters<DrawnFilter["fake"]["script"]>[1]>["fail"]>;

  /**
   * The editor of Spoilers with another name written, a script that fails the next save as `fail` says, and Save
   * pressed: the bar beside the button says `bar`.
   */
  async function saveRefused({ fail }: { fail: Failure }, bar: string | RegExp = marked) {
    const form = await filterForm({}, { filter: 0 });
    await rewrite(form.user, nameField(), "Spoilers and more");
    form.fake.script("PATCH /filters/{id}", { times: 1, fail });
    await refused(form, bar);
    return form;
  }

  test("the name (the story's server): the rule of a name under Name, which alone is marked, and what was written stays unsaved", async () => {
    const form = await filterForm({ settings: { filterSave: "title" } }, { filter: 0 });
    await rewrite(form.user, nameField(), "Spoilers\tand more");

    await refused(form);

    expect(invalid()).toEqual(["Name"]);
    expect(describedBy(nameField())).toEqual([nameHint, nameRule]);
    expect(said()).toEqual([marked]);
    expect(writes(form)).toEqual([`PATCH /filters/${form.id ?? ""}`]);
    expect(nameField().value).toBe("Spoilers\tand more");
    expect(form.told.includes("back")).toBe(false);
    expect(form.told.filter((word) => word.startsWith("unsaved")).at(-1)).toBe("unsaved:true");
  });

  test("a word, by its place among those sent: the rule of a word under that word alone", async () => {
    await saveRefused({ fail: refusal("keywords[1].keyword", "The server's own words.") });

    expect(invalid()).toEqual(["Word or phrase 2"]);
    expect(describedBy(wordField(2))).toEqual([wordRule]);
    expect(describedBy(wordField(1))).toEqual([]);
    expect(nameField().value).toBe("Spoilers and more");
  });

  test("a word the server names by its place among those sent is marked in its own row, a blank row before it not counted", async () => {
    const form = await filterForm({}, { filter: 0 });
    // A row left blank is not sent: the third word sent is the fourth row.
    await form.user.click(addWord());
    await form.user.click(addWord());
    await rewrite(form.user, wordField(4), "season 4");
    expect(words().map(([word]) => word)).toEqual(["finale", "season 3", "", "season 4"]);
    form.fake.script("PATCH /filters/{id}", { times: 1, fail: refusal("keywords[2].keyword", "Not this one.") });

    await refused(form);

    expect(invalid()).toEqual(["Word or phrase 4"]);
    expect(describedBy(wordField(4))).toEqual([wordRule]);
  });

  test("the places: the server's words describe every place", async () => {
    await saveRefused({ fail: refusal("contexts", "The server does not take these places.") });

    expect(placeNames.map((name) => describedBy(place(name)))).toEqual(
      placeNames.map(() => ["The server does not take these places."]),
    );
    expect(invalid()).toEqual([]);
  });

  test("the end: the server's words describe Ends", async () => {
    await saveRefused({ fail: refusal("expires_at", "The server does not take this end.") });

    expect(describedBy(endsChoice())).toEqual(["The server does not take this end."]);
  });

  /** What is wrong with the words as a whole stands once, under their list, which it describes; no word is marked or described. */
  function expectOfTheWords(sentence: string) {
    const [line, ...others] = section("Words").getAllByText(sentence);
    expect(others).toEqual([]);
    expect(describedBy(wordsList())).toEqual([sentence]);
    expect(
      line !== undefined && (wordsList().compareDocumentPosition(line) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0,
    ).toBe(true);
    expect(invalid()).toEqual([]);
    expect([1, 2].map((n) => describedBy(wordField(n)))).toEqual([[], []]);
    expect(describedBy(addWord())).toEqual([]);
  }

  test("the words as a whole: the server's words under the words describe their list, and no word is marked", async () => {
    const form = await filterForm({}, { filter: 0 });
    expect(describedBy(wordsList())).toEqual([]);
    // A filter saved unchanged sends nothing: a change of its name makes the request.
    await rewrite(form.user, nameField(), "Spoilers and more");
    form.fake.script("PATCH /filters/{id}", {
      times: 1,
      fail: refusal("keywords", "The server does not take these words."),
    });

    await refused(form);

    expectOfTheWords("The server does not take these words.");
  });

  test("what the filter does: the server's words under Posts that match describe the choice, shown once", async () => {
    await saveRefused({ fail: refusal("action", "The server does not take this action.") });

    expect(screen.queryAllByText("The server does not take this action.")).toHaveLength(1);
    expect(describedBy(actionChoice())).toEqual(["The server does not take this action."]);
    expect(describedBy(endsChoice())).toEqual([]);
  });

  test("more words than a filter or all the filters can have: the sentence of the limits under the words describes their list, and no word is marked", async () => {
    await saveRefused({
      fail: {
        type: "/api/v1/problems/too-many-filter-keywords",
        title: "You have as many words in your filters as you may",
        status: 422,
      },
    });

    expectOfTheWords("A filter has at most 100 words, and all your filters 1,000 in all. Remove some first.");
  });

  test("what the server refused is no longer said once anything of the editor is changed, another field than the one refused too", async () => {
    const form = await saveRefused({ fail: refusal("contexts", "The server does not take these places.") });
    expect(describedBy(place("Home and lists"))).toEqual(["The server does not take these places."]);

    await form.user.click(wholeWord(1));

    expect(placeNames.map((name) => describedBy(place(name)))).toEqual(placeNames.map(() => []));
    expect(screen.queryAllByText("The server does not take these places.")).toHaveLength(0);
    expect(alerts()).toEqual([]);
    // Nothing was sent by the change: the first Save alone.
    expect(writes(form)).toEqual([`PATCH /filters/${form.id ?? ""}`]);
  });

  test("more filters than a reader can have: the sentence of the limit beside the button, and nothing is marked", async () => {
    const form = await saveRefused(
      {
        fail: {
          type: "/api/v1/problems/too-many-keyword-filters",
          title: "You have as many filters as you may",
          status: 422,
        },
      },
      "You have 100 filters, the most you can have. Delete one to add another.",
    );

    expect(invalid()).toEqual([]);
    expect(form.told.includes("back")).toBe(false);
  });

  test("the hundred-and-first filter (the fake's own refusal): the sentence of the limit beside Add", async () => {
    const form = await filterForm({
      settings: { filters: Array.from({ length: 100 }, (_, i) => filterOf(`F${String(i)}`, 1)) },
    });
    await rewrite(form.user, nameField(), "One more");
    await rewrite(form.user, wordField(1), "more");

    await refused(form, "You have 100 filters, the most you can have. Delete one to add another.");

    expect(writes(form)).toEqual(["POST /filters"]);
    expect(invalid()).toEqual([]);
  });

  test("a refusal that names no field: the server's words beside the button, and nothing is marked", async () => {
    await saveRefused(
      { fail: { type: "about:blank", title: "This could not be done.", status: 422, detail: "The server's reason." } },
      "The server's reason.",
    );

    expect(invalid()).toEqual([]);
  });

  test("a save the server fails says so beside the button, and the next Save sends the filter again", async () => {
    const form = await filterForm({}, { filter: 0 });
    const sent = form.fake.script("PATCH /filters/{id}");
    const failing = form.fake.script("PATCH /filters/{id}", { times: 1, fail: 500 });
    await rewrite(form.user, nameField(), "Spoilers and more");

    // A problem of a status alone has a title of the fake's: the bar says something.
    await refused(form, /./);
    expect(invalid()).toEqual([]);
    expect(form.told.includes("back")).toBe(false);
    failing.end();

    await saved(form);

    expect(sent.seen).toHaveLength(2);
    expect(sent.seen[1]?.body).toEqual(sent.seen[0]?.body);
  });

  test("a save that gets no answer says that the filter could not be saved", async () => {
    await saveRefused({ fail: "network" }, "The filter could not be saved. Try again.");

    expect(invalid()).toEqual([]);
    expect(nameField().value).toBe("Spoilers and more");
  });

  test("what was refused is no longer said once the next Save is taken", async () => {
    const form = await saveRefused({ fail: refusal("keywords[0].keyword", "Not this one.") });
    expect(invalid()).toEqual(["Word or phrase 1"]);

    await saved(form);

    expect(invalid()).toEqual([]);
    expect(alerts()).toEqual([]);
    expect(describedBy(wordField(1))).toEqual([]);
  });

  // The bar says "fix what is marked": each sentence it means is marked for assistive technology too, as what
  // describes the part it is about. That part is a control, or for the words as a whole their list, which is no
  // control and holds no value to call wrong.
  describe("what the bar calls marked is a sentence that describes the part it is about", () => {
    /** How many parts the element saying `sentence` describes: the controls of the editor, and the lists. */
    const describes = (sentence: string) => {
      /** The id a part names the sentence by: the element's own, or that of the part it stands in. */
      const idOf = (line: HTMLElement | null): string => (line === null ? "" : line.id || idOf(line.parentElement));
      const ids = screen.queryAllByText(sentence).map((line) => idOf(line));
      return [
        ...screen.queryAllByRole("textbox"),
        ...screen.queryAllByRole("combobox"),
        ...screen.queryAllByRole("checkbox"),
        ...screen.queryAllByRole("button"),
        ...screen.queryAllByRole("list"),
      ].filter((control) =>
        (control.getAttribute("aria-describedby") ?? "").split(" ").some((id) => id !== "" && ids.includes(id)),
      ).length;
    };

    test("no place: each of the five places", async () => {
      const form = await filterForm({}, { filter: 0 });
      for (const name of placeNames) if (place(name).checked) await form.user.click(place(name));
      await refused(form);

      expect(describes("Choose at least one place.")).toBe(5);
    });

    // With no word and no row, the sentence describes a control.
    test("no word", async () => {
      const form = await filterForm({}, { filter: 0 });
      await form.user.click(removeWord(2));
      await form.user.click(removeWord(1));
      await refused(form);

      expect(describes("Add at least one word or phrase.")).toBeGreaterThan(0);
    });

    test("the words as a whole, refused by the server: the list of the words", async () => {
      await saveRefused({ fail: refusal("keywords", "The server does not take these words.") });

      expect(describes("The server does not take these words.")).toBe(1);
    });
  });
});

describe("Delete filter", () => {
  /** The confirmation of the deletion. */
  const asking = () =>
    screen.findByRole("alertdialog", { name: (name) => plain(name) === "Delete the filter “Spoilers”?" });
  const buttonsOf = (dialog: HTMLElement) =>
    within(dialog)
      .getAllByRole("button")
      .map((button) => button.getAttribute("aria-label") ?? button.textContent);

  test("Delete… asks first, in an alert dialog that names the filter and says that its words go with it and that it cannot be undone", async () => {
    const form = await filterForm({}, { filter: 0 });

    await form.user.click(screen.getByRole("button", { name: "Delete…" }));

    const dialog = await asking();
    expect(describedBy(dialog)).toEqual(["Its words are deleted with it. This cannot be undone."]);
    expect(buttonsOf(dialog)).toEqual(["Close", "Cancel", "Delete"]);
    expect(writes(form)).toEqual([]);
  });

  for (const button of ["Cancel", "Close"])
    test(`${button} closes the confirmation and deletes nothing`, async () => {
      const form = await filterForm({}, { filter: 0 });
      await form.user.click(screen.getByRole("button", { name: "Delete…" }));

      await form.user.click(within(await asking()).getByRole("button", { name: button }));

      await waitFor(() => {
        if (screen.queryAllByRole("alertdialog").length > 0) throw new Error("test: the confirmation is still open");
      });
      expect(writes(form)).toEqual([]);
      expect(form.told.includes("back")).toBe(false);
      expect(nameField().value).toBe("Spoilers");
    });

  test("Delete sends one DELETE of the filter, and tells the owner to go back; the server no longer holds it", async () => {
    const form = await filterForm({}, { filter: 0 });
    await form.user.click(screen.getByRole("button", { name: "Delete…" }));

    await form.user.click(within(await asking()).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      if (!form.told.includes("back")) throw new Error("test: the editor did not go back yet");
    });
    expect(writes(form)).toEqual([`DELETE /filters/${form.id ?? ""}`]);
    const { items } = await form.data.queryClient.query({ ...keywordFiltersQuery(form.data), staleTime: 0 });
    expect(items.map((filter) => filter.title)).toEqual(["Election season", "Marathon week"]);
  });

  test("a deletion the server fails keeps the filter and the editor, and says so", async () => {
    const form = await filterForm({}, { filter: 0 });
    form.fake.script("DELETE /filters/{id}", { times: 1, fail: 500 });
    await form.user.click(screen.getByRole("button", { name: "Delete…" }));

    await form.user.click(within(await asking()).getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      if (alerts().length === 0) throw new Error("test: nothing says that the deletion failed");
    });
    expect(form.told.includes("back")).toBe(false);
    const { items } = await form.data.queryClient.query({ ...keywordFiltersQuery(form.data), staleTime: 0 });
    expect(items.map((filter) => filter.title)).toEqual(["Spoilers", "Election season", "Marathon week"]);
  });
});

describe("the editor tells its owner whether it holds unsaved changes", () => {
  const last = (form: DrawnFilter) => form.told.filter((word) => word.startsWith("unsaved")).at(-1);

  test("none when it opens, some once something is written, and none again once it is put back", async () => {
    const form = await filterForm({}, { filter: 0 });
    expect(last(form)).not.toBe("unsaved:true");

    await form.user.type(nameField(), "!");
    await waitFor(() => {
      expect(last(form)).toBe("unsaved:true");
    });

    await form.user.type(nameField(), "{Backspace}");
    await waitFor(() => {
      expect(last(form)).toBe("unsaved:false");
    });
  });

  // Each kind of change: what the reader does to the editor of Spoilers.
  for (const [what, change] of [
    ["what it does to a post", (form: DrawnFilter) => form.user.selectOptions(actionChoice(), "Hide completely")],
    ["when it ends", (form: DrawnFilter) => form.user.selectOptions(endsChoice(), "In 1 day")],
    ["a word", (form: DrawnFilter) => rewrite(form.user, wordField(2), "season 4")],
    ["Whole word", (form: DrawnFilter) => form.user.click(wholeWord(2))],
    ["a word removed", (form: DrawnFilter) => form.user.click(removeWord(2))],
    ["a place", (form: DrawnFilter) => form.user.click(place("Profiles"))],
  ] as const)
    test(`some once the reader changed ${what}`, async () => {
      const form = await filterForm({}, { filter: 0 });

      await change(form);

      await waitFor(() => {
        expect(last(form)).toBe("unsaved:true");
      });
    });

  test("a save the server refuses leaves them unsaved", async () => {
    const form = await filterForm({}, { filter: 0 });
    form.fake.script("PATCH /filters/{id}", { times: 1, fail: 500 });
    await form.user.type(nameField(), "!");

    await refused(form, /./);

    expect(last(form)).toBe("unsaved:true");
    expect(nameField().value).toBe("Spoilers!");
  });
});

describe("leaving with unsaved changes", () => {
  /** The confirmation the editor shows while its owner says the reader is leaving. */
  const asking = () => screen.getByRole("alertdialog", { name: "Leave without saving?" });

  test("the editor asks first, in an alert dialog that says what is lost, with Keep editing and Leave", async () => {
    await filterForm({}, { filter: 0, leaving: true });

    expect(describedBy(asking())).toEqual(["The changes you have not saved will be lost. This cannot be undone."]);
    expect(
      within(asking())
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Close", "Keep editing", "Leave"]);
  });

  for (const [button, tells] of [
    ["Keep editing", "stay"],
    ["Close", "stay"],
    ["Leave", "leave"],
  ] as const)
    test(`${button} tells the owner to ${tells}`, async () => {
      const form = await filterForm({}, { leaving: true });

      await form.user.click(within(asking()).getByRole("button", { name: button }));

      expect(form.told.filter((word) => word === "stay" || word === "leave")).toEqual([tells]);
    });

  test("no confirmation shows while the reader is not leaving", async () => {
    const form = await filterForm({}, { filter: 0 });
    await form.user.type(nameField(), "!");

    expect(screen.queryAllByRole("alertdialog")).toHaveLength(0);
  });
});
