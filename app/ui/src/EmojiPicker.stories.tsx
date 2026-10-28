// SPDX-License-Identifier: AGPL-3.0-only
// EmojiPicker stories. The play functions test the arrow keys of the grid in the writing direction with the columns a
// real layout gives, the section buttons and aria-current following the scrolling, the ranked search and its announced
// count, the skin tone menu, picking with Shift held, and the popover. `FullSet` virtualizes every Unicode emoji of the
// app's data (app/web/src/features/emoji/unicode, served to Storybook by .storybook/main.ts) and moves through rows that
// were not rendered (ADR-0097).
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, waitFor } from "storybook/test";
import { Button } from "./Button";
import {
  EmojiPicker,
  type EmojiPickerLabels,
  type EmojiPickerProps,
  type EmojiPickerSection,
  type EmojiSectionIcon,
  type PickerCustomEmoji,
  type PickerUnicodeEmoji,
  type SkinTone,
} from "./EmojiPicker";
import { EmojiIcon } from "./Icons";
import { Popover, PopoverTrigger } from "./Popover";

// Plain squares stand in for emoji images, as in the CustomEmoji stories.
const orange =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGN4ViH3nxLMMGrAqAGjBgwXAwAvJnsfj06SzAAAAABJRU5ErkJggg==";
const blue =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAGUlEQVR4nGPQ6LnznxLMMGrAqAGjBgwXAwBfb48fVw/D1AAAAABJRU5ErkJggg==";

const blobcat: PickerCustomEmoji = { shortcode: "blobcat", url: blue, static_url: orange };
const thanks: PickerCustomEmoji = { shortcode: "thanks", url: blue, static_url: blue };

/** A Unicode emoji with its English name, keywords and, for a hand, its skin tone variants named the CLDR way. */
function unicode(emoji: string, name: string, keywords: readonly string[] = [], toned = false): PickerUnicodeEmoji {
  const tones = [
    ["\u{1F3FB}", "light skin tone"],
    ["\u{1F3FC}", "medium-light skin tone"],
    ["\u{1F3FD}", "medium skin tone"],
    ["\u{1F3FE}", "medium-dark skin tone"],
    ["\u{1F3FF}", "dark skin tone"],
  ] as const;
  return {
    unicode: emoji,
    name,
    keywords,
    ...(toned
      ? { skins: tones.map(([modifier, tone]) => ({ unicode: emoji + modifier, name: `${name}: ${tone}` })) }
      : {}),
  };
}

const thumbsUp = unicode("👍", "thumbs up", ["hand", "+1"], true);

const sections: EmojiPickerSection[] = [
  { id: "frequent", heading: "Frequently used", icon: "frequent", emojis: [blobcat, thumbsUp] },
  {
    id: "custom",
    heading: "Custom emoji",
    icon: "custom",
    emojis: [
      { shortcode: "wave", url: orange, static_url: orange, aliases: ["hello"] },
      thanks,
      blobcat,
      { shortcode: "blobfox", url: orange, static_url: orange, aliases: ["fox"] },
      { shortcode: "blobheart", url: blue, static_url: blue },
    ],
  },
  {
    id: "smileys-emotion",
    heading: "Smileys & emotion",
    icon: "smileys-emotion",
    emojis: [
      unicode("😀", "grinning face", ["face", "grin"]),
      unicode("😂", "face with tears of joy", ["laugh", "tears"]),
      unicode("🥰", "smiling face with hearts", ["love"]),
      unicode("🤔", "thinking face", ["thinking"]),
      unicode("😮", "face with open mouth", ["wow"]),
      unicode("😢", "crying face", ["sad", "tear"]),
      unicode("😇", "smiling face with halo", ["angel"]),
      unicode("❤️", "red heart", ["heart", "love"]),
    ],
  },
  {
    id: "people-body",
    heading: "People & body",
    icon: "people-body",
    emojis: [
      unicode("👋", "waving hand", ["hello", "wave", "hand"], true),
      unicode("✋", "raised hand", ["hand", "high 5"], true),
      thumbsUp,
      unicode("🙏", "folded hands", ["please", "thanks", "hand"], true),
      unicode("👀", "eyes", ["look"]),
      unicode("🧑‍💻", "technologist", ["coder", "computer"], true),
    ],
  },
  {
    id: "animals-nature",
    heading: "Animals & nature",
    icon: "animals-nature",
    emojis: [
      unicode("🐱", "cat face", ["cat", "pet"]),
      unicode("🦊", "fox", ["face"]),
      unicode("🐶", "dog face", ["dog", "pet"]),
      unicode("🌸", "cherry blossom", ["flower"]),
      unicode("🌳", "deciduous tree", ["tree"]),
      unicode("🍁", "maple leaf", ["leaf"]),
    ],
  },
  {
    id: "food-drink",
    heading: "Food & drink",
    icon: "food-drink",
    emojis: [
      unicode("🍣", "sushi", ["food"]),
      unicode("🍙", "rice ball", ["food"]),
      unicode("☕", "hot beverage", ["coffee", "tea"]),
    ],
  },
  {
    id: "travel-places",
    heading: "Travel & places",
    icon: "travel-places",
    emojis: [unicode("🗻", "mount fuji", ["mountain"]), unicode("✈️", "airplane", ["plane"])],
  },
  {
    id: "activities",
    heading: "Activities",
    icon: "activities",
    emojis: [unicode("🎉", "party popper", ["party", "tada"]), unicode("⚽", "soccer ball", ["ball"])],
  },
  {
    id: "objects",
    heading: "Objects",
    icon: "objects",
    emojis: [unicode("💡", "light bulb", ["idea"]), unicode("📚", "books", ["book"])],
  },
  {
    id: "symbols",
    heading: "Symbols",
    icon: "symbols",
    emojis: [unicode("💯", "hundred points", ["100"]), unicode("🔥", "fire", ["flame"])],
  },
  {
    id: "flags",
    heading: "Flags",
    icon: "flags",
    emojis: [unicode("🏁", "chequered flag", ["racing"]), unicode("🇯🇵", "flag: Japan", ["flag"])],
  },
];

const labels: EmojiPickerLabels = {
  search: "Search emoji",
  grid: "Emoji",
  categories: "Emoji categories",
  results: "Search results",
  resultCount: (count) => (count === 1 ? "1 emoji found" : `${String(count)} emoji found`),
  empty: "No emoji found",
  skinTone: "Skin tone",
  skinTones: {
    none: "Default skin tone",
    light: "Light skin tone",
    "medium-light": "Medium-light skin tone",
    medium: "Medium skin tone",
    "medium-dark": "Medium-dark skin tone",
    dark: "Dark skin tone",
  },
};

/** The picker with the skin tone kept in state, as the app keeps it. */
function TonedPicker(props: EmojiPickerProps) {
  const [tone, setTone] = useState<SkinTone>(props.skinTone ?? "none");
  return (
    <EmojiPicker
      {...props}
      skinTone={tone}
      onSkinToneChange={(next) => {
        setTone(next);
        props.onSkinToneChange?.(next);
      }}
    />
  );
}

const meta = {
  title: "Forms/EmojiPicker",
  component: EmojiPicker,
  args: { sections, onSelect: fn(), onSkinToneChange: fn(), labels },
  render: (args) => <TonedPicker {...args} />,
  decorators: [
    (Story) => (
      // As wide as the popover of the app at most (ADR-0091 §4: 22 rem).
      <div className="max-w-88">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof EmojiPicker>;

export default meta;
type Story = StoryObj<typeof meta>;

/**
 * The sections in order, under their buttons; Tab goes from the search field to the skin tone button, the buttons
 * and the grid, the arrow keys move in the writing direction, and Enter picks.
 */
export const Grid: Story = {
  play: async ({ canvas, userEvent, args }) => {
    // The first of the rows named so: the frequent ones come first.
    const rowOf = (name: string) => canvas.getAllByRole("gridcell", { name })[0];
    await userEvent.click(canvas.getByRole("searchbox", { name: "Search emoji" }));
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Skin tone" })).toHaveFocus();
    await userEvent.tab();
    await expect(canvas.getByRole("button", { name: "Frequently used" })).toHaveFocus();
    // On Tab, the toolbar moves focus to its last button and lets the browser's Tab go on from there (React Aria's
    // Toolbar); user-event goes on from the button the key was pressed on instead, so start from the last one.
    canvas.getByRole("button", { name: "Flags" }).focus();
    await userEvent.tab();
    await expect(rowOf(":blobcat:")).toHaveFocus();
    // The first emoji of the next section is below the frequent one.
    await userEvent.keyboard("{ArrowDown}");
    await expect(rowOf(":wave:")).toHaveFocus();
    const forward = document.documentElement.dir === "rtl" ? "{ArrowLeft}" : "{ArrowRight}";
    await userEvent.keyboard(forward);
    await expect(rowOf(":thanks:")).toHaveFocus();
    await userEvent.keyboard("{ArrowUp}");
    await expect(rowOf("thumbs up")).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await expect(args.onSelect).toHaveBeenLastCalledWith({
      kind: "unicode",
      emoji: thumbsUp,
      text: "👍",
      keepOpen: false,
    });
  },
};

/**
 * A section button scrolls its heading to the top and moves focus to its first emoji; the button of the section at
 * the top of the grid has aria-current="true", and scrolling moves it.
 */
export const SectionButtons: Story = {
  play: async ({ canvas, userEvent }) => {
    const toolbar = canvas.getByRole("toolbar", { name: "Emoji categories" });
    const button = (name: string) => canvas.getByRole("button", { name });
    await expect(button("Frequently used")).toHaveAttribute("aria-current", "true");
    await userEvent.click(button("Animals & nature"));
    await expect(canvas.getByRole("gridcell", { name: "cat face" })).toHaveFocus();
    // The heading scrolls to the top of the grid, and the button of its section is the current one once it is there.
    await waitFor(async () => {
      await expect(button("Animals & nature")).toHaveAttribute("aria-current", "true");
      await expect(button("Frequently used")).not.toHaveAttribute("aria-current");
    });
    // Scrolling back to the top moves it back.
    const grid = canvas.getByRole("grid", { name: "Emoji" });
    grid.scrollTop = 0;
    await waitFor(async () => {
      await expect(button("Frequently used")).toHaveAttribute("aria-current", "true");
      await expect(button("Animals & nature")).not.toHaveAttribute("aria-current");
    });
    await expect(toolbar).toBeVisible();
  },
};

/**
 * A search ranks the emoji of every section into one section: name prefixes, then keyword prefixes, then substrings.
 * The section buttons are disabled, and the count is announced once typing pauses.
 */
export const Searched: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(canvas.getByRole("searchbox", { name: "Search emoji" }), "hand");
    await expect(canvas.getByRole("rowheader", { name: "Search results" })).toBeVisible();
    const names = canvas.getAllByRole("gridcell").map((cell) => cell.getAttribute("aria-label"));
    await expect(names).toEqual(["thumbs up", "waving hand", "raised hand", "folded hands"]);
    await expect(canvas.getByRole("button", { name: "People & body" })).toBeDisabled();
    await waitFor(() => expect(canvas.getByRole("status")).toHaveTextContent("4 emoji found"));
  },
};

/** Nothing matches the search. */
export const NoMatch: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(canvas.getByRole("searchbox", { name: "Search emoji" }), "nothing");
    await expect(canvas.getByText("No emoji found")).toBeVisible();
    await waitFor(() => expect(canvas.getByRole("status")).toHaveTextContent("0 emoji found"));
  },
};

/** The skin tone menu: the emoji with variants change to the tone chosen. */
export const SkinTones: Story = {
  play: async ({ canvas, userEvent, args }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Skin tone" }));
    await userEvent.click(await canvas.findByRole("menuitemradio", { name: "Medium-dark skin tone" }));
    await expect(args.onSkinToneChange).toHaveBeenCalledWith("medium-dark");
    await expect(canvas.getByRole("gridcell", { name: "waving hand: medium-dark skin tone" })).toHaveTextContent("👋🏾");
    await waitFor(() => expect(canvas.queryByRole("menu")).not.toBeInTheDocument());
    await userEvent.click(
      canvas.getAllByRole("gridcell", { name: "thumbs up: medium-dark skin tone" })[0] as HTMLElement,
    );
    await expect(args.onSelect).toHaveBeenLastCalledWith({
      kind: "unicode",
      emoji: thumbsUp,
      text: "👍🏾",
      keepOpen: false,
    });
  },
};

/** The skin tone menu open, the tone chosen checked. */
export const SkinToneMenu: Story = {
  args: { skinTone: "light" },
  decorators: [
    (Story) => (
      <div className="min-h-120">
        <Story />
      </div>
    ),
  ],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Skin tone" }));
    await expect(await canvas.findByRole("menuitemradio", { name: "Light skin tone" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  },
};

/** Picking with Shift held asks the app to keep the picker open, and focus stays on the emoji. */
export const KeepOpen: Story = {
  play: async ({ canvas, userEvent, args }) => {
    const thinking = canvas.getByRole("gridcell", { name: "thinking face" });
    await userEvent.keyboard("{Shift>}");
    await userEvent.click(thinking);
    await userEvent.keyboard("{/Shift}");
    await expect(args.onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ text: "🤔", keepOpen: true }));
    await expect(thinking).toHaveFocus();
    await userEvent.keyboard("{Shift>}{Enter}{/Shift}");
    await expect(args.onSelect).toHaveBeenCalledTimes(2);
    await expect(args.onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ text: "🤔", keepOpen: true }));
  },
};

/** Only custom emoji in one section: no section buttons, and without the skin tone handler no skin tone button. */
export const CustomOnly: Story = {
  args: { sections: [{ id: "custom", heading: "Custom emoji", icon: "custom", emojis: sections[1]?.emojis ?? [] }] },
  render: (args) => <EmojiPicker sections={args.sections} onSelect={args.onSelect} labels={args.labels} />,
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("toolbar")).not.toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: "Skin tone" })).not.toBeInTheDocument();
  },
};

/** Animated emoji stay still when the user turns animations off. */
export const StillOnly: Story = { args: { allowMotion: false } };

/** In a popover behind an icon button, as the post form has it: the search field takes focus when it opens. */
export const InPopover: Story = {
  decorators: [
    (Story) => (
      <div className="min-h-120">
        <Story />
      </div>
    ),
  ],
  render: (args) => (
    <PopoverTrigger>
      <Button variant="ghost" size="toolbar" aria-label="Add emoji">
        <EmojiIcon />
      </Button>
      <Popover aria-label="Emoji">
        {({ close }) => (
          <TonedPicker
            {...args}
            autoFocus
            onSelect={(selection) => {
              args.onSelect(selection);
              if (!selection.keepOpen) close();
            }}
          />
        )}
      </Popover>
    </PopoverTrigger>
  ),
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Add emoji" }));
    await waitFor(() => expect(canvas.getByRole("searchbox", { name: "Search emoji" })).toHaveFocus());
    // Esc closes it and focus returns to the trigger. The screenshot is of the closed state: an open popover with
    // a blinking caret and its entering animation does not render the same twice (the other stories show the
    // picker itself).
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(canvas.queryByRole("searchbox")).not.toBeInTheDocument());
    await waitFor(() => expect(canvas.getByRole("button", { name: "Add emoji" })).toHaveFocus());
  },
};

/** The groups of the app's base data (base.gen.json), as far as this story reads them. */
type BaseData = {
  groups: readonly {
    id: EmojiSectionIcon;
    emoji: readonly (readonly [string, string, (readonly (readonly [string, string])[])?])[];
  }[];
};
/** The English names of the app's data (en.gen.json). */
type NameData = {
  emoji: Readonly<Record<string, readonly [string, readonly string[]]>>;
  skins: Readonly<Record<string, string>>;
};

/** The heading of each group in English, as the app's catalog has them. */
const groupHeadings: Readonly<Record<string, string>> = {
  "smileys-emotion": "Smileys & emotion",
  "people-body": "People & body",
  "animals-nature": "Animals & nature",
  "food-drink": "Food & drink",
  "travel-places": "Travel & places",
  activities: "Activities",
  objects: "Objects",
  symbols: "Symbols",
  flags: "Flags",
};

/** Reads one of the app's data files that .storybook/main.ts serves under /unicode-emoji/. */
async function readData(file: string): Promise<unknown> {
  // eslint-disable-next-line no-restricted-globals -- a story reads the app's generated data, which @my/ui cannot import (web-client/README.md A2)
  const response = await fetch(`/unicode-emoji/${file}`);
  return response.json();
}

/** Every Unicode emoji of the app's data, by group, named in English. */
async function loadFullSet(): Promise<EmojiPickerSection[]> {
  const [base, names] = (await Promise.all([readData("base.gen.json"), readData("en.gen.json")])) as [
    BaseData,
    NameData,
  ];
  return base.groups.map((group) => ({
    id: group.id,
    heading: groupHeadings[group.id] ?? group.id,
    icon: group.id,
    emojis: group.emoji.map(([emoji, , skins]) => {
      const [name, keywords] = names.emoji[emoji] ?? [emoji, []];
      return {
        unicode: emoji,
        name,
        keywords,
        ...(skins === undefined
          ? {}
          : { skins: skins.map(([variant]) => ({ unicode: variant, name: names.skins[variant] ?? variant })) }),
      };
    }),
  }));
}

/**
 * Every Unicode emoji (about 1,900 without the skin tone variants), virtualized (ADR-0097): only the rows in view and
 * four on each side are rendered, the grid tells its whole size, and the keys, the section buttons and focus work
 * across rows that were not rendered. It has no screenshots, since the emoji fonts load as rows scroll into view; the
 * other stories carry the picker's screenshots.
 */
export const FullSet: Story = {
  tags: ["no-screenshot"],
  loaders: [async () => ({ fullSet: await loadFullSet() })],
  render: (args, { loaded }) => <TonedPicker {...args} sections={loaded["fullSet"] as EmojiPickerSection[]} />,
  play: async ({ canvas, userEvent }) => {
    const grid = canvas.getByRole("grid", { name: "Emoji" });
    const rowCount = Number(grid.getAttribute("aria-rowcount"));
    const columns = Number(grid.getAttribute("aria-colcount"));
    // Every emoji is in a row of the count, but only a few rows are rendered.
    await expect(rowCount * columns).toBeGreaterThan(1900);
    await expect(canvas.getAllByRole("gridcell").length).toBeLessThan(columns * 16);
    // A section button scrolls far down, smoothly when motion is allowed, and focuses the section's first emoji.
    await userEvent.click(canvas.getByRole("button", { name: "Flags" }));
    await waitFor(() => expect(canvas.getByRole("gridcell", { name: "chequered flag" })).toHaveFocus());
    await waitFor(() => expect(canvas.getByRole("button", { name: "Flags" })).toHaveAttribute("aria-current", "true"), {
      timeout: 5000,
    });
    await expect(canvas.queryByRole("gridcell", { name: "grinning face" })).not.toBeInTheDocument();
    // The last emoji of the grid, in the last row.
    await userEvent.keyboard("{Control>}{End}{/Control}");
    const last = canvas.getAllByRole("gridcell").at(-1) as HTMLElement;
    await expect(last).toHaveFocus();
    await expect(last.closest("[role=row]")).toHaveAttribute("aria-rowindex", String(rowCount));
    // Back to the first; the grid scrolls to the top, where the focused emoji is.
    await userEvent.keyboard("{Control>}{Home}{/Control}");
    await waitFor(() => expect(canvas.getByRole("gridcell", { name: "grinning face" })).toHaveFocus());
    await waitFor(() =>
      expect(canvas.getByRole("button", { name: "Smileys & emotion" })).toHaveAttribute("aria-current", "true"),
    );
    // Page Down moves by the rows in view, in the same column.
    await userEvent.keyboard("{PageDown}");
    const paged = document.activeElement;
    await expect(paged).toHaveAttribute("role", "gridcell");
    await expect(paged).toHaveAttribute("aria-colindex", "1");
    // Scrolled away with the wheel, the focused emoji stays rendered and focused.
    grid.scrollTop = grid.scrollHeight;
    await waitFor(() => expect(canvas.queryByRole("gridcell", { name: "grinning face" })).not.toBeInTheDocument());
    await expect(paged).toBeInTheDocument();
    await expect(paged).toHaveFocus();
  },
};
