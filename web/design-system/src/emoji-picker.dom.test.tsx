// SPDX-License-Identifier: AGPL-3.0-only

// The emoji picker's contract: a search field, a skin tone menu, a toolbar of categories and a grid of emoji that the
// arrows move in, which draws only the rows near its view however many emoji it holds, while it tells the size of the
// whole (ADR-0091 §14).

import { expect, jest, test } from "bun:test";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EmojiPicker, type EmojiPickerProps, type EmojiSection, type PickerEmoji } from "./emoji-picker";

/** `n` emoji named `<prefix> <i>`, each its own; the first has skin tones, `<prefix>0-1` to `<prefix>0-5`. */
const many = (n: number, prefix: string): PickerEmoji[] =>
  Array.from({ length: n }, (_, i) => ({
    key: `${prefix}${String(i)}`,
    name: `${prefix} ${String(i)}`,
    text: `${prefix}${String(i)}`,
    ...(i === 0 ? { tones: [1, 2, 3, 4, 5].map((tone) => `${prefix}0-${String(tone)}`) } : {}),
  }));

const blobcat: PickerEmoji = {
  key: ":blobcat:",
  name: ":blobcat:",
  image: { url: "https://m.example/blobcat.webp", staticUrl: "https://m.example/blobcat.png" },
};

/** Two sections of twenty emoji, the second with a custom emoji in a group of its own. */
const sections: EmojiSection[] = [
  { id: "faces", label: "Faces", icon: "emojiSmileys", emojis: many(20, "face") },
  {
    id: "people",
    label: "People",
    icon: "emojiPeople",
    emojis: many(20, "hand"),
    groups: [{ label: "Blobs", emojis: [blobcat] }],
  },
];

const everyEmoji = sections.flatMap((section) => [
  ...section.emojis,
  ...(section.groups ?? []).flatMap((g) => g.emojis),
]);
const search = (query: string) => everyEmoji.filter((emoji) => emoji.name.startsWith(query));
const labels = { tone: "Skin tone", tones: ["Default", "Light", "Medium-light", "Medium", "Medium-dark", "Dark"] };

type Picked = [key: string, text: string, keep: boolean];

/** The picker of the two sections between two buttons of the page, and what was picked, in order. */
function picker(props: Partial<EmojiPickerProps> = {}) {
  const picked: Picked[] = [];
  render(
    <>
      <button type="button">Before</button>
      <EmojiPicker
        sections={sections}
        search={search}
        labels={labels}
        onSelect={(emoji, text, { keep }) => picked.push([emoji.key, text, keep])}
        {...props}
      />
      <button type="button">After</button>
    </>,
  );
  return picked;
}

const cell = (name: string) => screen.getByRole("gridcell", { name });

/** Puts the focus on the grid's first emoji from the keyboard: the page's button, the field, the tone, the categories. */
async function tabToGrid(user: ReturnType<typeof userEvent.setup>) {
  for (let stop = 0; stop < 5; stop++) await user.tab();
  expect(document.activeElement).toBe(cell("face 0"));
}

test("the picker is a search field, a skin tone button, a toolbar of categories and a grid of the emoji by section", () => {
  picker();

  expect(screen.getByRole("searchbox", { name: "Search emoji" })).not.toBeNull();
  expect(screen.getByRole("button", { name: "Skin tone: Default", expanded: false })).not.toBeNull();
  const categories = screen.getByRole("toolbar", { name: "Emoji categories" });
  expect(within(categories).getAllByRole("button")).toEqual([
    screen.getByRole("button", { name: "Faces" }),
    screen.getByRole("button", { name: "People" }),
  ]);
  const grid = screen.getByRole("grid", { name: "Emoji" });
  const faces = within(grid).getByRole("rowgroup", { name: "Faces" });
  expect(within(faces).getByRole("rowheader", { name: "Faces" })).not.toBeNull();
  expect(within(faces).getAllByRole("gridcell")).toHaveLength(20);
  const people = within(grid).getByRole("rowgroup", { name: "People" });
  expect(within(people).getByRole("rowheader", { name: "Blobs" })).not.toBeNull();
  expect(within(people).getByRole("gridcell", { name: ":blobcat:" })).not.toBeNull();
});

test("the grid draws a bounded number of cells however many emoji it holds, and tells the size of the whole", () => {
  picker({
    sections: [
      { id: "a", label: "A", icon: "emojiSmileys", emojis: many(2000, "a") },
      { id: "b", label: "B", icon: "emojiPeople", emojis: many(2000, "b") },
    ],
  });
  const grid = screen.getByRole("grid", { name: "Emoji" });
  const columns = Number(grid.getAttribute("aria-colcount"));

  const cells = within(grid).getAllByRole("gridcell").length;
  // A popover's most height of rows in view, and four rows on each side.
  expect(cells).toBeGreaterThan(0);
  expect(cells).toBeLessThanOrEqual(columns * 16);
  // Two headings and 4000 emoji in rows of as many columns as it says.
  expect(columns).toBeGreaterThan(0);
  expect(Number(grid.getAttribute("aria-rowcount"))).toBe(2 + 2 * Math.ceil(2000 / columns));
  // Each row says where it is among them.
  expect(within(grid).getByRole("rowheader", { name: "A" }).parentElement?.getAttribute("aria-rowindex")).toBe("1");
  expect(cell("a 0").getAttribute("aria-colindex")).toBe("1");
  expect(cell("a 1").getAttribute("aria-colindex")).toBe("2");
});

test("Tab stops once at the field, the skin tone, the categories and the grid", async () => {
  const user = userEvent.setup();
  picker();

  const stops: (Element | null)[] = [];
  for (let stop = 0; stop < 6; stop++) {
    await user.tab();
    stops.push(document.activeElement);
  }

  expect(stops).toEqual([
    screen.getByRole("button", { name: "Before" }),
    screen.getByRole("searchbox", { name: "Search emoji" }),
    screen.getByRole("button", { name: "Skin tone: Default" }),
    screen.getByRole("button", { name: "Faces" }),
    cell("face 0"),
    screen.getByRole("button", { name: "After" }),
  ]);
});

test("the field takes the focus when the picker appears, if asked", () => {
  picker({ autoFocus: true });

  expect(document.activeElement).toBe(screen.getByRole("searchbox", { name: "Search emoji" }));
});

test("the arrows move the focus by an emoji and by a row, and past a row's end to the next row", async () => {
  const user = userEvent.setup();
  picker();
  const columns = Number(screen.getByRole("grid", { name: "Emoji" }).getAttribute("aria-colcount"));
  await tabToGrid(user);

  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(cell("face 1"));
  await user.keyboard("{ArrowDown}");
  expect(document.activeElement).toBe(cell(`face ${String(1 + columns)}`));
  await user.keyboard("{ArrowLeft}");
  expect(document.activeElement).toBe(cell(`face ${String(columns)}`));
  await user.keyboard("{ArrowLeft}");
  expect(document.activeElement).toBe(cell(`face ${String(columns - 1)}`));
  await user.keyboard("{ArrowRight}{ArrowUp}");
  expect(document.activeElement).toBe(cell("face 0"));
});

test("Home and End move the focus to the ends of a row, and with Control to the ends of the grid", async () => {
  const user = userEvent.setup();
  picker();
  const columns = Number(screen.getByRole("grid", { name: "Emoji" }).getAttribute("aria-colcount"));
  await tabToGrid(user);

  await user.keyboard("{ArrowRight}{End}");
  expect(document.activeElement).toBe(cell(`face ${String(columns - 1)}`));
  await user.keyboard("{Home}");
  expect(document.activeElement).toBe(cell("face 0"));
  await user.keyboard("{Control>}{End}{/Control}");
  expect(document.activeElement).toBe(cell(":blobcat:"));
  await user.keyboard("{Control>}{Home}{/Control}");
  expect(document.activeElement).toBe(cell("face 0"));
});

test("Enter and Space pick the focused emoji, which keeps the focus; with Shift they say to keep picking", async () => {
  const user = userEvent.setup();
  const picked = picker();
  await tabToGrid(user);
  await user.keyboard("{ArrowRight}");

  await user.keyboard("{Enter}");
  await user.keyboard(" ");
  await user.keyboard("{Shift>}{Enter}{/Shift}");
  await user.keyboard("{Shift>} {/Shift}");

  expect(picked).toEqual([
    ["face1", "face1", false],
    ["face1", "face1", false],
    ["face1", "face1", true],
    ["face1", "face1", true],
  ]);
  expect(document.activeElement).toBe(cell("face 1"));
});

test("a press picks an emoji, with Shift held to keep picking; a custom emoji is picked as its shortcode", async () => {
  const user = userEvent.setup();
  const picked = picker();

  await user.click(cell("hand 3"));
  await user.keyboard("{Shift>}");
  await user.click(cell("hand 4"));
  await user.keyboard("{/Shift}");
  await user.click(cell(":blobcat:"));

  expect(picked).toEqual([
    ["hand3", "hand3", false],
    ["hand4", "hand4", true],
    [":blobcat:", ":blobcat:", false],
  ]);
});

test("the emoji the reader reacted with says so in its name", () => {
  picker({ current: "face2" });

  expect(cell("face 2, your reaction")).not.toBeNull();
  expect(screen.queryByRole("gridcell", { name: "face 2" })).toBeNull();
});

test("the skin tone button opens a menu of the six tones, the one in use checked, and a choice tells the tone", async () => {
  const user = userEvent.setup();
  const tones: number[] = [];
  picker({ tone: 3, onToneChange: (tone) => tones.push(tone) });

  await user.click(screen.getByRole("button", { name: "Skin tone: Medium", expanded: false }));

  // Named by its own label: its button's name says the tone in use too.
  const menu = await screen.findByRole("menu", { name: "Skin tone" });
  expect(within(menu).getAllByRole("menuitemradio")).toHaveLength(6);
  expect(within(menu).getByRole("menuitemradio", { name: "Medium", checked: true })).not.toBeNull();
  await user.click(within(menu).getByRole("menuitemradio", { name: "Medium-dark", checked: false }));

  expect(tones).toEqual([4]);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: /^Skin tone: / }));
});

test("an emoji with skin tones is picked in the tone in use, one without them as it is", async () => {
  const user = userEvent.setup();
  const picked = picker({ tone: 4 });
  expect(screen.getByRole("button", { name: "Skin tone: Medium-dark" })).not.toBeNull();

  await user.click(cell("face 0"));
  await user.click(cell("face 1"));

  expect(picked).toEqual([
    ["face0", "face0-4", false],
    ["face1", "face1", false],
  ]);
});

test("the arrows move among the categories, and a category's button takes the focus to its first emoji", async () => {
  const user = userEvent.setup();
  picker();
  expect(screen.getByRole("button", { name: "Faces", current: true })).not.toBeNull();
  await user.tab();
  await user.tab();
  await user.tab();
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Faces" }));

  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "People" }));
  await user.keyboard("{Enter}");

  expect(document.activeElement).toBe(cell("hand 0"));
});

test("a search shows its results in the sections' place, the categories off, until the field is cleared", async () => {
  const user = userEvent.setup();
  const picked = picker();

  await user.click(screen.getByRole("searchbox", { name: "Search emoji" }));
  await user.keyboard("hand 1");

  const grid = screen.getByRole("grid", { name: "Emoji" });
  const results = within(grid).getByRole("rowgroup", { name: "Search results" });
  // hand 1, and hand 10 to hand 19.
  expect(within(results).getAllByRole("gridcell")).toHaveLength(11);
  expect(within(grid).getAllByRole("rowgroup")).toEqual([results]);
  expect(Number(grid.getAttribute("aria-rowcount"))).toBe(
    1 + Math.ceil(11 / Number(grid.getAttribute("aria-colcount"))),
  );
  for (const name of ["Faces", "People"])
    expect(screen.getByRole<HTMLButtonElement>("button", { name }).disabled).toBe(true);

  await user.click(cell("hand 12"));
  expect(picked).toEqual([["hand12", "hand12", false]]);

  await user.clear(screen.getByRole("searchbox", { name: "Search emoji" }));
  expect(within(grid).getByRole("rowgroup", { name: "Faces" })).not.toBeNull();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "Faces" }).disabled).toBe(false);
});

/**
 * What the picker says to assistive technology of the search it opens with, once the moment it waits for the typing
 * to rest has passed: the test's clock passes it, not the test.
 */
function saidOfSearch(query: string) {
  jest.useFakeTimers();
  try {
    picker({ defaultQuery: query });
    act(() => {
      jest.advanceTimersByTime(2000);
    });
    return screen.getByRole("status").textContent;
  } finally {
    jest.useRealTimers();
  }
}

test("a search says how many emoji it found", () => {
  expect(saidOfSearch("hand 1")).toBe("11 emoji found");
});

test("a search that matches no emoji says so in the grid and to assistive technology", () => {
  expect(saidOfSearch("xylophone")).toBe("No emoji found");

  const grid = screen.getByRole("grid", { name: "Emoji" });
  expect(within(grid).getByRole("gridcell").textContent).toBe("No emoji match “xylophone”. Try another word.");
});

test("ArrowDown from a row to a shorter one under a group's heading lands on its last emoji, and ArrowUp returns over the heading", async () => {
  const user = userEvent.setup();
  picker();
  const columns = Number(screen.getByRole("grid", { name: "Emoji" }).getAttribute("aria-colcount"));
  // People: twenty hands in rows of as many columns, then the group Blobs, a row of one emoji.
  const lastRow = Math.floor(19 / columns) * columns;
  expect(columns).toBeGreaterThan(1);
  expect(lastRow + 1).toBeLessThan(20);
  for (let stop = 0; stop < 4; stop++) await user.tab();
  await user.keyboard("{ArrowRight}{Enter}");
  expect(document.activeElement).toBe(cell("hand 0"));
  for (let row = 0; row < lastRow / columns; row++) await user.keyboard("{ArrowDown}");
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(cell(`hand ${String(lastRow + 1)}`));

  await user.keyboard("{ArrowDown}");
  expect(document.activeElement).toBe(cell(":blobcat:"));

  await user.keyboard("{ArrowUp}");
  expect(document.activeElement).toBe(cell(`hand ${String(lastRow)}`));
});
