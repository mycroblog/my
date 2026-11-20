// SPDX-License-Identifier: AGPL-3.0-only

// Relationships, the entry of the settings (FR-303, FR-304, ADR-0110 §5), on the fake server as yuki reads it: the
// accounts they block (the scenario's quickdeals, and the account sora left behind), the account they mute (kai, its
// notifications too), two servers they mute and two hashtags they follow; with every list empty, each saying what it
// would hold and how something gets there; with nine muted servers, of which the entry shows five and the door to the
// whole list, and seven hashtags, shown whole; the whole list of the muted servers, the screen under the entry; the
// whole list of more blocked accounts than a page holds, before and after Show more; and while it loads, and when it
// could not be read.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { manyBlocked } from "./pages.samples";

const at = "/settings/relationships";

/** The servers and hashtags of the entry's first story, beside the scenario's blocks and mute. */
const server = { domainMutes: ["spam.example", "ads.example"], followedTags: ["baking", "photography"] } as const;

/**
 * Nine muted servers, more than the entry shows whole (seven), and seven followed hashtags, which it shows whole.
 */
const many = {
  domainMutes: [
    "spam.example",
    "ads.example",
    "deals.example",
    "clicks.example",
    "noise.example",
    "loud.example",
    "bots.example",
    "promo.example",
    "flood.example",
  ],
  followedTags: ["baking", "photography", "hiking", "sourdough", "film", "mountains", "rivers"],
} as const;

const meta = {
  title: "Settings relationships",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at, server }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * /settings/relationships: two blocked accounts, one muted account whose notifications are hidden too, two muted
 * servers and two followed hashtags, each list with its count, each row with the button that takes it out.
 */
export const Entry: Story = {};
/** Every list empty: each says what it would hold and how something comes to be in it. */
export const Empty: Story = { parameters: screen({ at, state: "empty" }) };
/** Nine muted servers, the first five and the door to the whole list; seven followed hashtags, all of them. */
export const Many: Story = { parameters: screen({ at, server: many }) };
/** /settings/relationships/servers: the whole list of the muted servers, the screen under the entry. */
export const AllServers: Story = { parameters: screen({ at: `${at}/servers`, server: many }) };
/** The whole list of more blocked accounts than a page holds: Show more under the first twenty. */
export const AllBlockedMany: Story = { parameters: screen({ at: `${at}/blocked`, server: manyBlocked }) };
/** The same after Show more: the rest under the first twenty, and no Show more. */
export const AllBlockedManyMore: Story = {
  parameters: screen({ at: `${at}/blocked`, server: manyBlocked, more: true }),
};
/** While the lists are on their way: the entry's head, and that it loads. */
export const Loading: Story = { parameters: screen({ at, state: "loading" }) };
/** When none of the lists could be read: it says so, with the way to try again. */
export const Failed: Story = { parameters: screen({ at, state: "error" }) };
