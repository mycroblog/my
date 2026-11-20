// SPDX-License-Identifier: AGPL-3.0-only

// The custom emoji (FR-111, FR-915, ADR-0051, ADR-0070), on the fake server (app.samples.tsx; staff-emoji.ts) as the
// administrator (hana) reads them: the list under the tabs Emoji | Requests, the waiting requests counted on the
// second; kept to this server's or to others', found by a shortcode, with nothing at all, loading, failing, and for a
// moderator, whom it is not open to. An emoji's page: a local one with a category and aliases, one the picker leaves
// out, a disabled one, another server's (Copy to this server), one whose image cannot be shown; loading, failing, and
// an address that names none. A new emoji's form. The requests: the waiting ones, those of another state are the
// filter's own, none waiting, loading, failing. What follows a press (a request opened over the list, approved or
// refused) is the route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration/Custom emoji",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/custom-emojis", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the administrator reads it. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });
/** The page of the fake's emoji of `key`. */
const emoji = (key: string, more: Omit<Fake, "at"> = {}) => at(`/admin/custom-emojis/{server:emoji:${key}}`, more);

/** Every emoji, newest first: its image, :shortcode:, category and aliases or server, and whether it is shown. */
export const Emoji: Story = { parameters: at("/admin/custom-emojis") };
/** This server's emoji alone. */
export const EmojiOfThisServer: Story = { parameters: at("/admin/custom-emojis?origin=local") };
/** Other servers' emoji alone: each its domain. */
export const EmojiOfOtherServers: Story = { parameters: at("/admin/custom-emojis?origin=remote") };
/** A shortcode that no emoji holds: the list says so, with Clear filters. */
export const EmojiNoMatch: Story = { parameters: at("/admin/custom-emojis?shortcode=nothing") };
/** A server with no custom emoji at all. */
export const EmojiNone: Story = {
  parameters: at("/admin/custom-emojis", { server: { staffServer: { emoji: { none: true } } } }),
};
export const EmojiLoading: Story = { parameters: at("/admin/custom-emojis", { state: "loading" }) };
export const EmojiError: Story = { parameters: at("/admin/custom-emojis", { state: "error" }) };
/** The address, for a moderator, whose permissions do not open it. */
export const EmojiNotOpen: Story = { parameters: screen({ at: "/admin/custom-emojis", viewer: "marco" }) };

/** A local emoji with a category and aliases: Disable and a new image first, what it is, its place in the picker, Delete last. */
export const LocalEmoji: Story = { parameters: emoji("wave") };
/** A local emoji the picker leaves out. */
export const EmojiNotInThePicker: Story = { parameters: emoji("staff_only") };
/** A disabled local emoji, made with the command line: Enable in the place of Disable. */
export const DisabledEmoji: Story = { parameters: emoji("old_logo") };
/** A local emoji a request made: who asked for it. */
export const EmojiFromARequest: Story = { parameters: emoji("mio_heart") };
/** Another server's emoji: its server and address, and Copy to this server under a shortcode. */
export const RemoteEmoji: Story = { parameters: emoji("blobfox") };
/** Another server's emoji whose image this server cannot show: no copy. */
export const RemoteEmojiWithoutImage: Story = { parameters: emoji("hidden_media") };
export const EmojiPageLoading: Story = { parameters: emoji("wave", { state: "loading" }) };
export const EmojiPageError: Story = { parameters: emoji("wave", { state: "error" }) };
/** An address that names no emoji. */
export const EmojiMissing: Story = { parameters: at("/admin/custom-emojis/none") };

/** A new local emoji: its shortcode, its image and its place in the picker. */
export const NewEmoji: Story = { parameters: at("/admin/custom-emojis/new") };

/** The requests that wait: mio's two, and one of kai's, who may no longer create emoji. */
export const Requests: Story = { parameters: at("/admin/custom-emojis/requests") };
/** No request waits: the list says so, and the tab counts none. */
export const RequestsNone: Story = {
  parameters: at("/admin/custom-emojis/requests", { server: { staffServer: { emoji: { none: true } } } }),
};
export const RequestsLoading: Story = { parameters: at("/admin/custom-emojis/requests", { state: "loading" }) };
export const RequestsError: Story = { parameters: at("/admin/custom-emojis/requests", { state: "error" }) };
