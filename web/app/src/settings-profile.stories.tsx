// SPDX-License-Identifier: AGPL-3.0-only

// Profile, the entry of the settings (FR-006, ADR-0110 §5), and Edit profile, the screen under it, on the fake server
// as yuki reads them. The entry, where every control saves at once: as the scenario's reader has it, with the badges
// they may show and the hashtags they feature; as a new account has it, with neither; with eight badges, one list of
// them; on a server that tells search engines not to list any account and refuses AI for every account, where the
// search switch stays the reader's and its line says what it still decides, and the AI switch is disabled, shows what
// the server enforces and says why; on one that decides a part of each, where the lines say what the choice still
// does; and while it loads, and when it could not be read. The form, saved as a whole: filled, and with nothing yet;
// on a server that refuses the display name, which Save then shows beside the field; on one that refuses every
// upload, which Save shows under the image picked; on one whose review refuses the change; and on one that never
// answers, where Save stays busy. What a control of the entry says of a change (saving, saved, not saved) is the
// design system's own story (Saving), and the fields of an image and of a pair in their states are theirs (Picture
// field, Pair field).
// The reader's own profile with the action that leads to the form is Profile's first story (Own).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const entry = "/settings/profile";
const edit = "/settings/profile/edit";

const meta = {
  title: "Settings profile",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: entry }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** /settings/profile: the profile as others see it with Edit profile, two featured hashtags, and two badges. */
export const Entry: Story = {};
/** The entry as a new account has it: no hashtag featured, and no badge, so no Badges section. */
export const EntryNew: Story = {
  parameters: screen({ at: entry, server: { settings: { badges: [], featured: [] } } }),
};
/** The entry of an account with eight badges: one list of them in the Badges section, a switch each. */
export const EntryManyBadges: Story = {
  parameters: screen({
    at: entry,
    server: {
      settings: {
        badges: [
          { label: "Administrator", color: "neutral", displayed: true },
          { label: "Moderator", color: "neutral", displayed: true },
          { label: "Supporter", color: "neutral", displayed: false },
          { label: "Translator", color: "neutral", displayed: true },
          { label: "Emoji maker", color: "neutral", displayed: false },
          { label: "Event host", color: "neutral", displayed: false },
          { label: "Founding member", color: "neutral", displayed: false },
          { label: "Volunteer of the year 2026", color: "neutral", displayed: false },
        ],
      },
    },
  }),
};
/**
 * The entry on a server that tells search engines not to list any account, has no search of posts, and refuses AI for
 * every account: the search switch stays the reader's own choice, and its line says that other servers still receive
 * it; the AI switch is disabled and on, with the sentence that the server already refuses both.
 */
export const EntryServerDecides: Story = {
  parameters: screen({ at: entry, server: { crawlers: { search: "deny", ai: "deny" } } }),
};
/**
 * The entry on a server that tells search engines not to list any account and has a search of posts, and refuses AI
 * training alone: both choices still do something on this server, and their lines say what.
 */
export const EntryServerDecidesPart: Story = {
  parameters: screen({
    at: entry,
    server: { crawlers: { search: "deny", ai: "deny_training" }, search: { posts: true } },
  }),
};
/** The entry while the reader's badges and featured hashtags are on their way: its head, and that it loads. */
export const EntryLoading: Story = { parameters: screen({ at: entry, state: "loading" }) };
/** The entry when they could not be read: it says so, with the way to try again. */
export const EntryFailed: Story = { parameters: screen({ at: entry, state: "error" }) };
/** /settings/profile/edit: the form with the reader's images, name, bio and one field. */
export const Edit: Story = { parameters: screen({ at: edit }) };
/** The form of an account with nothing yet: no image, no name, no bio, and no field, where the list would be. */
export const EditEmpty: Story = { parameters: screen({ at: edit, server: { profile: { start: "empty" } } }) };
/** The form on a server that refuses the display name: after a change of it, Save says so beside the field. */
export const EditRefused: Story = { parameters: screen({ at: edit, server: { profile: { save: "name" } } }) };
/** The form on a server whose review refuses the change: Save says so in the submit bar. */
export const EditRejected: Story = { parameters: screen({ at: edit, server: { profile: { save: "refused" } } }) };
/** The form on a server that never answers: Save stays busy, and nothing can be changed meanwhile. */
export const EditSaving: Story = { parameters: screen({ at: edit, server: { profile: { save: "waits" } } }) };
/** The form on a server that refuses every upload: after an image is picked, Save says so under that image. */
export const EditPictureRefused: Story = { parameters: screen({ at: edit, server: { uploads: "refused" } }) };
