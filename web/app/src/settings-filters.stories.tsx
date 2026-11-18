// SPDX-License-Identifier: AGPL-3.0-only

// Keyword filters, the entry of the settings (FR-209, ADR-0157 §6, §9, §10), and a filter's editor, the screen under
// it, on the fake server as yuki reads them. The entry: the fake's three filters, one that folds, one that hides and
// ends in a month, and one that ended, still listed as expired; with none, where what stands in the list's place leads
// to adding one; and while it loads, and when it could not be read. The editor, saved as a whole: a new filter; the
// filter that folds; the one that ends, whose end is kept unless another is chosen; the expired one; on a server that
// refuses the name, which Save then shows under the field; on one that never answers, where Save stays busy; a filter
// at the most words it can have, where adding stops and says why; a new filter of a reader whose other filters come
// near the most words of all, which says how many more it can take; and an address that names no filter.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const entry = "/settings/filters";
const fresh = "/settings/filters/new";

/** A filter that folds the posts of its words at home, with `count` words. */
const filterOf = (title: string, count: number) => ({
  title,
  action: "warn" as const,
  contexts: ["home" as const],
  keywords: Array.from({ length: count }, (_, i) => ({
    keyword: `${title.toLowerCase()} ${String(i + 1)}`,
    whole_word: false,
  })),
});

const meta = {
  title: "Settings filters",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: entry }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * /settings/filters: Spoilers, which folds, Election season, which hides and ends in a month, and Marathon week,
 * which ended and is marked Expired; each row says what it does, where, how many words, and when it ends.
 */
export const Entry: Story = {};
/** No filter yet: the sentence of what a filter does, and Add a filter in the list's place. */
export const EntryEmpty: Story = { parameters: screen({ at: entry, server: { settings: { filters: [] } } }) };
/** While the filters are on their way: the entry's head, and that it loads. */
export const EntryLoading: Story = { parameters: screen({ at: entry, state: "loading" }) };
/** When the filters could not be read: it says so, with the way to try again. */
export const EntryFailed: Story = { parameters: screen({ at: entry, state: "error" }) };
/** /settings/filters/new: a new filter, one empty word, every place checked, folding, never ending. */
export const New: Story = { parameters: screen({ at: fresh }) };
/** The editor of Spoilers: its name, two words (one whole word), three places, folding, never ending. */
export const Edit: Story = { parameters: screen({ at: "filter:0" }) };
/** The editor of Election season, which hides and has an end: its end is kept unless another is chosen. */
export const EditEnding: Story = { parameters: screen({ at: "filter:1" }) };
/** The editor of Marathon week, which ended: keeping its end keeps it expired, another end brings it back. */
export const EditExpired: Story = { parameters: screen({ at: "filter:2" }) };
/**
 * On a server that refuses the name: after a change and Save, the refusal stands under the name, which takes the
 * focus. (Save on the filter as it is sends nothing and goes back to the list.)
 */
export const EditRefused: Story = {
  parameters: screen({ at: "filter:0", server: { settings: { filterSave: "title" } } }),
};
/** On a server that never answers: after a change, Save stays busy, and nothing can be changed meanwhile. */
export const EditSaving: Story = {
  parameters: screen({ at: "filter:0", server: { settings: { filterSave: "waits" } } }),
};
/** A filter of 100 words, the most it can have: Add a word is disabled, and the line under the words says why. */
export const EditAtLimit: Story = {
  parameters: screen({ at: "filter:0", server: { settings: { filters: [filterOf("Full", 100)] } } }),
};
/**
 * A new filter of a reader whose other filters have 993 words: one word in it, and the line under the words says how
 * many more all the filters can take.
 */
export const NewNearLimit: Story = {
  parameters: screen({
    at: fresh,
    server: {
      settings: {
        filters: [
          ...Array.from({ length: 9 }, (_, i) => filterOf(`Topic ${String(i + 1)}`, 100)),
          filterOf("Last", 93),
        ],
      },
    },
  }),
};
/** An address that names no filter: it says so, with the way to the list. */
export const NotFound: Story = { parameters: screen({ at: `${entry}/nothing` }) };
