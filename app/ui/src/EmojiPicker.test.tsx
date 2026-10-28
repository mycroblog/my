// SPDX-License-Identifier: AGPL-3.0-only
// jsdom lays nothing out, so the picker lays its grid out by its fallback metrics (ADR-0097 §1): 8 columns, emoji rows
// of 44 px, heading rows of 28 px, in a grid 256 px high. The stories check the layout in Chromium.
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { I18nProvider } from "react-aria-components";
import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";
import {
  EmojiPicker,
  type EmojiPickerLabels,
  type EmojiPickerSection,
  normalizeEmojiSearch,
  type PickerCustomEmoji,
  type PickerUnicodeEmoji,
  searchEmojis,
  type SkinTone,
  skinToneVariant,
} from "./EmojiPicker";
import { countAnnounceDelay } from "./TextArea";

const still = "https://media.example/still.webp";
const moving = "https://media.example/moving.webp";

const wave: PickerCustomEmoji = { shortcode: "wave", url: still, static_url: still, aliases: ["hello"] };
const blobcat: PickerCustomEmoji = { shortcode: "blobcat", url: moving, static_url: still };
const blobfox: PickerCustomEmoji = { shortcode: "blobfox", url: still, static_url: still, aliases: ["fox"] };

const wavingHand: PickerUnicodeEmoji = {
  unicode: "👋",
  name: "waving hand",
  keywords: ["hello", "wave"],
  skins: [
    { unicode: "👋🏻", name: "waving hand: light skin tone" },
    { unicode: "👋🏽", name: "waving hand: medium skin tone" },
  ],
};
const handshake: PickerUnicodeEmoji = {
  unicode: "🤝",
  name: "handshake",
  keywords: ["agreement", "hand"],
  skins: [
    { unicode: "🫱🏽‍🫲🏻", name: "handshake: medium skin tone, light skin tone" },
    { unicode: "🤝🏽", name: "handshake: medium skin tone" },
  ],
};
const thumbsUp: PickerUnicodeEmoji = { unicode: "👍", name: "thumbs up", keywords: ["hand", "+1"] };
const fox: PickerUnicodeEmoji = { unicode: "🦊", name: "きつね", keywords: ["どうぶつ", "フォックス", "fox"] };

const sections: EmojiPickerSection[] = [
  { id: "recent", heading: "Recently used", icon: "frequent", emojis: [blobcat] },
  { id: "custom", heading: "Blobs", icon: "custom", emojis: [wave, blobcat, blobfox] },
  { id: "people", heading: "People", icon: "people-body", emojis: [wavingHand, handshake, thumbsUp] },
  { id: "animals", heading: "Animals", icon: "animals-nature", emojis: [fox] },
];

/** Nine sections of 100 emoji each, "emoji 0" to "emoji 899": far more rows than the grid shows. */
const manySections: EmojiPickerSection[] = Array.from({ length: 9 }, (_, section) => ({
  id: `section-${String(section + 1)}`,
  heading: `Section ${String(section + 1)}`,
  icon: "symbols",
  emojis: Array.from({ length: 100 }, (_, index) => {
    const number = section * 100 + index;
    return { unicode: `e${String(number)}`, name: `emoji ${String(number)}` };
  }),
}));

const tones: Record<SkinTone, string> = {
  none: "Default",
  light: "Light",
  "medium-light": "Medium-light",
  medium: "Medium",
  "medium-dark": "Medium-dark",
  dark: "Dark",
};

const labels: EmojiPickerLabels = {
  search: "Search emoji",
  grid: "Emoji",
  categories: "Categories",
  results: "Search results",
  resultCount: (count) => `${String(count)} emoji found`,
  empty: "No emoji found",
  skinTone: "Skin tone",
  skinTones: tones,
};

/** The name of each heading and emoji of the grid, in order. */
function rowNames(): string[] {
  return [...screen.getAllByRole("rowheader"), ...screen.getAllByRole("gridcell")]
    .toSorted((first, second) => (first.compareDocumentPosition(second) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1))
    .map((cell) => cell.getAttribute("aria-label") ?? cell.textContent);
}

/** The emoji of the grid named so (each name is once in `manySections`). */
function cell(name: string): HTMLElement {
  return screen.getByRole("gridcell", { name });
}

describe("EmojiPicker", () => {
  // The virtualizer scrolls no further than the grid's scrollHeight - clientHeight, which jsdom gives as 0: the grid
  // here is 256 px high, and its content as high as the block size the picker gives its rows.
  const sizes = {
    scrollHeight: (element: HTMLElement) => {
      // eslint-disable-next-line testing-library/no-node-access -- a layout stub, not a query: it reads the block size of the grid's content
      const content = element.firstElementChild;
      return content instanceof HTMLElement ? Number.parseFloat(content.style.blockSize) || 0 : 0;
    },
    clientHeight: () => 256,
  };
  const originals = new Map<string, PropertyDescriptor | undefined>();
  beforeAll(() => {
    for (const [property, size] of Object.entries(sizes)) {
      originals.set(property, Object.getOwnPropertyDescriptor(HTMLElement.prototype, property));
      Object.defineProperty(HTMLElement.prototype, property, {
        configurable: true,
        get(this: HTMLElement) {
          return this.getAttribute("role") === "grid" ? size(this) : 0;
        },
      });
    }
  });
  afterAll(() => {
    for (const [property, descriptor] of originals) {
      if (descriptor === undefined) Reflect.deleteProperty(HTMLElement.prototype, property);
      else Object.defineProperty(HTMLElement.prototype, property, descriptor);
    }
  });

  it("shows the sections in order, each emoji named by its shortcode or its name", () => {
    render(<EmojiPicker sections={sections} onSelect={() => {}} labels={labels} />);
    expect(screen.getByRole("grid", { name: "Emoji" })).toBeInTheDocument();
    expect(rowNames()).toEqual([
      "Recently used",
      ":blobcat:",
      "Blobs",
      ":wave:",
      ":blobcat:",
      ":blobfox:",
      "People",
      "waving hand",
      "handshake",
      "thumbs up",
      "Animals",
      "きつね",
    ]);
    expect(screen.getAllByRole("img", { name: ":blobcat:" })).toHaveLength(2);
    expect(screen.getAllByRole("rowgroup").map((group) => group.getAttribute("aria-label"))).toEqual(["Recently used", "Blobs", "People", "Animals"]);
    const toolbar = screen.getByRole("toolbar", { name: "Categories" });
    expect(within(toolbar).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["Recently used", "Blobs", "People", "Animals"]);
    expect(within(toolbar).getByRole("button", { name: "Recently used" })).toHaveAttribute("aria-current", "true");
  });

  it("goes from the search field to the skin tone, the section buttons and the grid with Tab, and through the grid with the keys", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<EmojiPicker sections={sections} onSelect={onSelect} labels={labels} skinTone="none" onSkinToneChange={() => {}} autoFocus />);
    expect(screen.getByRole("searchbox", { name: "Search emoji" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "Skin tone" })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "Recently used" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Blobs" })).toHaveFocus();
    await user.tab();
    const [recent] = screen.getAllByRole("gridcell", { name: ":blobcat:" });
    expect(recent).toHaveFocus();
    // The grid's ends with Control.
    await user.keyboard("{Control>}{End}{/Control}");
    expect(cell("きつね")).toHaveFocus();
    await user.keyboard("{Control>}{Home}{/Control}");
    expect(recent).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "custom", emoji: blobcat, keepOpen: false });
    // Along the order, on into the next section.
    await user.keyboard("{ArrowRight}");
    expect(cell(":wave:")).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getAllByRole("gridcell", { name: ":blobcat:" })[1]).toHaveFocus();
    await user.keyboard(" ");
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "custom", emoji: blobcat, keepOpen: false });
    // The row's ends without Control.
    await user.keyboard("{End}");
    expect(cell(":blobfox:")).toHaveFocus();
    await user.keyboard("{Home}");
    expect(cell(":wave:")).toHaveFocus();
    // Down and up go by rows past the headings, in the same column or the row's last.
    await user.keyboard("{End}{ArrowDown}");
    expect(cell("thumbs up")).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(cell("きつね")).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(cell("きつね")).toHaveFocus();
    await user.keyboard("{ArrowUp}{ArrowUp}");
    expect(cell(":wave:")).toHaveFocus();
    // One cell of the grid is in the tab sequence: the one that had focus last.
    expect(screen.getAllByRole("gridcell").filter((gridcell) => gridcell.tabIndex === 0)).toEqual([cell(":wave:")]);
  });

  it("moves the arrow keys along the writing direction", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider locale="ar-XB">
        <EmojiPicker sections={sections} onSelect={() => {}} labels={labels} />
      </I18nProvider>,
    );
    await user.click(screen.getByRole("searchbox", { name: "Search emoji" }));
    await user.tab();
    await user.tab();
    await user.keyboard("{ArrowLeft}");
    expect(cell(":wave:")).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getAllByRole("gridcell", { name: ":blobcat:" })[0]).toHaveFocus();
  });

  it("moves focus to the first emoji of a section with its button", async () => {
    const user = userEvent.setup();
    render(<EmojiPicker sections={sections} onSelect={() => {}} labels={labels} />);
    const toolbar = screen.getByRole("toolbar", { name: "Categories" });
    await user.click(within(toolbar).getByRole("button", { name: "People" }));
    expect(cell("waving hand")).toHaveFocus();
    // The grid cannot scroll the heading to its top, but the section is the one moved to.
    expect(within(toolbar).getByRole("button", { name: "People" })).toHaveAttribute("aria-current", "true");
    expect(within(toolbar).getByRole("button", { name: "Recently used" })).not.toHaveAttribute("aria-current");
  });

  it("renders only the rows in view and a few beyond, and tells the size of the grid and the place of each row and cell", () => {
    render(<EmojiPicker sections={manySections} onSelect={() => {}} labels={labels} />);
    const grid = screen.getByRole("grid", { name: "Emoji" });
    // Per section a heading row and 13 rows of 8 columns.
    expect(grid).toHaveAttribute("aria-rowcount", "126");
    expect(grid).toHaveAttribute("aria-colcount", "8");
    // The heading and 6 rows are in view, and 4 more below.
    expect(screen.getAllByRole("rowheader").map((header) => header.textContent)).toEqual(["Section 1"]);
    expect(screen.getAllByRole("gridcell")).toHaveLength(80);
    expect(screen.queryByRole("gridcell", { name: "emoji 80" })).not.toBeInTheDocument();
    const rows = screen.getAllByRole("row");
    expect(rows.map((row) => row.getAttribute("aria-rowindex"))).toEqual(["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"]);
    expect(within(rows[2] as HTMLElement).getAllByRole("gridcell").map((gridcell) => gridcell.getAttribute("aria-colindex"))).toEqual(["1", "2", "3", "4", "5", "6", "7", "8"]);
    expect(cell("emoji 9")).toHaveAttribute("aria-colindex", "2");
  });

  it("moves focus to rows that were not rendered, across sections, and keeps the focused emoji when it scrolls away", async () => {
    const user = userEvent.setup();
    render(<EmojiPicker sections={manySections} onSelect={() => {}} labels={labels} />);
    const grid = screen.getByRole("grid", { name: "Emoji" });
    await user.click(cell("emoji 0"));
    // As many rows as the grid shows (256 / 44 = 5), in the same column.
    await user.keyboard("{ArrowRight}{PageDown}");
    expect(cell("emoji 41")).toHaveFocus();
    await user.keyboard("{PageUp}");
    expect(cell("emoji 1")).toHaveFocus();
    // Down past the last row of the first section (emoji 96 to 99) and its heading, into the next section.
    await user.keyboard("{ArrowDown>12/}");
    expect(cell("emoji 97")).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(cell("emoji 101")).toHaveFocus();
    expect(screen.getByRole("rowgroup", { name: "Section 2" })).toContainElement(cell("emoji 101"));
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(cell("emoji 99")).toHaveFocus();
    await user.keyboard("{Control>}{End}{/Control}");
    const last = cell("emoji 899");
    expect(last).toHaveFocus();
    const lastRow = screen.getAllByRole("row").at(-1) as HTMLElement;
    expect(lastRow).toHaveAttribute("aria-rowindex", "126");
    expect(within(lastRow).getByRole("gridcell", { name: "emoji 899" })).toBe(last);
    expect(screen.queryByRole("gridcell", { name: "emoji 0" })).not.toBeInTheDocument();
    // Scrolled back to the top, the focused emoji stays rendered and focused, apart from the rows in view.
    grid.scrollTop = 0;
    fireEvent.scroll(grid);
    expect(cell("emoji 0")).toBeInTheDocument();
    expect(last).toBeInTheDocument();
    expect(last).toHaveFocus();
    expect(last).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("rowgroup", { name: "Section 9" })).toContainElement(last);
  });

  it("scrolls a far section's heading to the top with its button and moves focus to its first emoji", async () => {
    const user = userEvent.setup();
    render(<EmojiPicker sections={manySections} onSelect={() => {}} labels={labels} />);
    const toolbar = screen.getByRole("toolbar", { name: "Categories" });
    await user.click(within(toolbar).getByRole("button", { name: "Section 5" }));
    expect(cell("emoji 400")).toHaveFocus();
    // Four sections of 28 + 13 × 44 px above it.
    expect(screen.getByRole("grid", { name: "Emoji" }).scrollTop).toBe(2400);
    expect(screen.getByRole("rowheader", { name: "Section 5" })).toBeInTheDocument();
    expect(within(toolbar).getByRole("button", { name: "Section 5" })).toHaveAttribute("aria-current", "true");
    expect(within(toolbar).getByRole("button", { name: "Section 1" })).not.toHaveAttribute("aria-current");
  });

  it("ranks the search results in one section, disables the section buttons and announces the count once typing pauses", async () => {
    // Testing Library waits on real timeouts inside act(), so the fake clock also follows real time (as in TextArea's tests).
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    try {
      const user = userEvent.setup({ delay: null });
      render(<EmojiPicker sections={sections} onSelect={() => {}} labels={labels} />);
      const status = screen.getByRole("status");
      await user.type(screen.getByRole("searchbox", { name: "Search emoji" }), "Hand");
      // A name prefix, then a keyword prefix, then a substring of a name.
      expect(rowNames()).toEqual(["Search results", "handshake", "thumbs up", "waving hand"]);
      expect(screen.getByRole("grid", { name: "Emoji" })).toHaveAttribute("aria-rowcount", "2");
      for (const button of within(screen.getByRole("toolbar", { name: "Categories" })).getAllByRole("button")) {
        expect(button).toBeDisabled();
        expect(button).not.toHaveAttribute("aria-current");
      }
      expect(status).toHaveTextContent("");
      act(() => {
        vi.advanceTimersByTime(countAnnounceDelay);
      });
      expect(status).toHaveTextContent("3 emoji found");
      await user.clear(screen.getByRole("searchbox", { name: "Search emoji" }));
      expect(status).toHaveTextContent("");
      expect(screen.getByRole("button", { name: "People" })).toBeEnabled();
      await user.type(screen.getByRole("searchbox", { name: "Search emoji" }), "nothing");
      expect(screen.getByText("No emoji found")).toBeInTheDocument();
      act(() => {
        vi.advanceTimersByTime(countAnnounceDelay);
      });
      expect(status).toHaveTextContent("0 emoji found");
    } finally {
      vi.useRealTimers();
    }
  });

  it("searches every emoji, rendered or not, and starts the results at the top", async () => {
    const user = userEvent.setup();
    render(<EmojiPicker sections={manySections} onSelect={() => {}} labels={labels} />);
    await user.click(within(screen.getByRole("toolbar", { name: "Categories" })).getByRole("button", { name: "Section 9" }));
    await user.type(screen.getByRole("searchbox", { name: "Search emoji" }), "emoji 89");
    expect(screen.getByRole("grid", { name: "Emoji" }).scrollTop).toBe(0);
    // The exact name, then the names that start with it.
    expect(rowNames()).toEqual(["Search results", "emoji 89", ...Array.from({ length: 10 }, (_, index) => `emoji ${String(890 + index)}`)]);
    // The section buttons are disabled, so Tab goes straight to the grid, and to its first result.
    await user.tab();
    expect(cell("emoji 89")).toHaveFocus();
  });

  it("puts a frequent emoji once among the results, and finds a custom emoji by its shortcode with colons or an alias", async () => {
    const user = userEvent.setup();
    render(<EmojiPicker sections={sections} onSelect={() => {}} labels={labels} />);
    const search = screen.getByRole("searchbox", { name: "Search emoji" });
    await user.type(search, ":blob");
    expect(rowNames()).toEqual(["Search results", ":blobcat:", ":blobfox:"]);
    await user.clear(search);
    await user.type(search, "fox");
    // The alias of :blobfox: and the keyword of 🦊 start with it; :blobfox: is given first.
    expect(rowNames()).toEqual(["Search results", ":blobfox:", "きつね"]);
  });

  it("shows and picks the emoji with variants in the skin tone chosen, the same for every person", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const onSkinToneChange = vi.fn();
    const { rerender } = render(<EmojiPicker sections={sections} onSelect={onSelect} labels={labels} skinTone="none" onSkinToneChange={onSkinToneChange} />);
    expect(cell("waving hand")).toHaveTextContent("👋");
    await user.click(screen.getByRole("button", { name: "Skin tone" }));
    const menu = screen.getByRole("menu", { name: "Skin tone" });
    expect(within(menu).getByRole("menuitemradio", { name: "Default" })).toHaveAttribute("aria-checked", "true");
    await user.click(within(menu).getByRole("menuitemradio", { name: "Medium" }));
    expect(onSkinToneChange).toHaveBeenCalledWith("medium");
    rerender(<EmojiPicker sections={sections} onSelect={onSelect} labels={labels} skinTone="medium" onSkinToneChange={onSkinToneChange} />);
    expect(cell("waving hand: medium skin tone")).toHaveTextContent("👋🏽");
    expect(cell("handshake: medium skin tone")).toHaveTextContent("🤝🏽");
    // Without a variant, an emoji stays as it is.
    expect(cell("thumbs up")).toHaveTextContent("👍");
    await user.click(cell("handshake: medium skin tone"));
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "unicode", emoji: handshake, text: "🤝🏽", keepOpen: false });
  });

  it("has no skin tone button without its handler", () => {
    render(<EmojiPicker sections={sections} onSelect={() => {}} labels={labels} />);
    expect(screen.queryByRole("button", { name: "Skin tone" })).not.toBeInTheDocument();
  });

  it("asks to stay open when an emoji is picked with Shift held, and focus stays in the grid", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<EmojiPicker sections={sections} onSelect={onSelect} labels={labels} />);
    await user.keyboard("{Shift>}");
    await user.click(cell("handshake"));
    await user.keyboard("{/Shift}");
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "unicode", emoji: handshake, text: "🤝", keepOpen: true });
    expect(cell("handshake")).toHaveFocus();
    await user.keyboard("{ArrowRight}{Shift>}{Enter}{/Shift}");
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "unicode", emoji: thumbsUp, text: "👍", keepOpen: true });
    expect(cell("thumbs up")).toHaveFocus();
    await user.keyboard("{ArrowLeft}{Enter}");
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "unicode", emoji: handshake, text: "🤝", keepOpen: false });
  });

  it("does not stay open for a touch, Shift or not", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<EmojiPicker sections={sections} onSelect={onSelect} labels={labels} />);
    await user.keyboard("{Shift>}");
    await user.pointer({ keys: "[TouchA]", target: cell("thumbs up") });
    await user.keyboard("{/Shift}");
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "unicode", emoji: thumbsUp, text: "👍", keepOpen: false });
  });
});

describe("searchEmojis", () => {
  it("compares in NFKC, lower case and hiragana", () => {
    expect(normalizeEmojiSearch(" ：ＦＯＸ： ")).toBe("fox");
    expect(normalizeEmojiSearch("フォックス")).toBe("ふぉっくす");
    expect(searchEmojis([thumbsUp, fox], "ﾌｫｯｸｽ")).toEqual([fox]);
    expect(searchEmojis([thumbsUp, fox], "キツネ")).toEqual([fox]);
  });

  it("ranks the exact name and the emoji itself first, then name prefixes, keyword prefixes and substrings", () => {
    expect(searchEmojis([thumbsUp, wavingHand, handshake], "hand")).toEqual([handshake, thumbsUp, wavingHand]);
    expect(searchEmojis([wavingHand, thumbsUp], "👍")).toEqual([thumbsUp]);
    expect(searchEmojis([blobfox, wave, blobcat], "b")).toEqual([blobfox, blobcat]);
    expect(searchEmojis([wave, blobcat], "")).toEqual([]);
  });
});

describe("skinToneVariant", () => {
  it("takes the variant whose every modifier is the tone", () => {
    expect(skinToneVariant(handshake, "medium")?.unicode).toBe("🤝🏽");
    expect(skinToneVariant(handshake, "light")).toBeUndefined();
    expect(skinToneVariant(wavingHand, "none")).toBeUndefined();
    expect(skinToneVariant(thumbsUp, "dark")).toBeUndefined();
  });
});
