// SPDX-License-Identifier: AGPL-3.0-only

// Custom emoji, the entry of the settings at /settings/custom-emojis (FR-915, ADR-0070 §2–§4), and the screens under
// it, on the fake server (staff-emoji.ts) as mio reads them, whose role lets them ask for emoji: what they used of
// the month's limit with Request an emoji, their requests in every state (waiting with Withdraw, approved, not
// approved with the reason, withdrawn), and the emoji they made; at the month's limit (why, in Request's place); with
// no limit; with nothing yet (yuki given the same role); loading, failing. The menu offers it only to such a reader:
// mio without the role has no entry, and the administrator, who registers emoji from the administration, has none
// either. Under it: the form of a request, and the form at the limit; one of their emoji, and an address that names
// none of theirs.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const entry = "/settings/custom-emojis";

/** The screen at `path` as mio reads it, holding the role that lets them ask for emoji. */
const asMio = (path: string, more: Omit<Fake, "at" | "viewer"> = {}) =>
  screen({
    at: path,
    viewer: "mio",
    ...more,
    server: { ...more.server, staffServer: { ...more.server?.staffServer, roles: { viewer: ["artist"] } } },
  });

const meta = {
  title: "Settings custom emoji",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...asMio(entry) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The entry: what mio asked this month of their 5, two waiting; the requests in every state; their emoji. */
export const Entry: Story = {};
/** At the month's limit: why, and when they may ask again, in the place of Request an emoji. */
export const AtTheLimit: Story = { parameters: asMio(entry, { server: { staffServer: { emoji: { limit: 2 } } } }) };
/** No monthly limit for the reader. */
export const NoLimit: Story = { parameters: asMio(entry, { server: { staffServer: { emoji: { limit: "none" } } } }) };
/** A reader with the role and nothing yet: sentences in the lists' place. */
export const NothingYet: Story = {
  parameters: screen({ at: entry, viewer: "yuki", server: { staffServer: { roles: { viewer: ["artist"] } } } }),
};
export const Loading: Story = { parameters: asMio(entry, { state: "loading" }) };
export const Failed: Story = { parameters: asMio(entry, { state: "error" }) };
/** The menu of a reader who may ask for emoji: Custom emoji among the general entries. */
export const MenuWithTheEntry: Story = { parameters: asMio("/settings") };
/** The administrator's menu: no Custom emoji, since they register emoji from the administration. */
export const MenuOfTheAdministrator: Story = { parameters: screen({ at: "/settings", viewer: "hana" }) };

/** A new request: a shortcode and an image. */
export const Request: Story = { parameters: asMio(`${entry}/new`) };
/** The request's address at the month's limit: why, and no form. */
export const RequestAtTheLimit: Story = {
  parameters: asMio(`${entry}/new`, { server: { staffServer: { emoji: { limit: 2 } } } }),
};
/** One of mio's emoji: its image, its place in the picker, and Delete last. */
export const OwnEmoji: Story = { parameters: asMio(`${entry}/{server:emoji:mio_heart}`) };
/** An address that names no emoji of mio's. */
export const OwnEmojiMissing: Story = { parameters: asMio(`${entry}/{server:emoji:wave}`) };
