// SPDX-License-Identifier: AGPL-3.0-only

// The settings (ADR-0110 §5), which the navigation's Settings leads to: the menu at /settings, the screen of its entry
// Display and posting, with the menu beside it from the frame's `wide` and its head with the way back under it, the
// confirmation of signing out, and an address that names no entry. What a control of Display and posting says of a
// change (saving, saved, not saved) is the design system's own story (Settings parts, the save states of a control);
// here the account's choices show as the server holds them.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const menu = "/settings";
const display = "/settings/display";

const meta = {
  title: "Settings",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: menu }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** /settings: the menu, the entries in their groups, and Sign out… apart at its end. */
export const Menu: Story = {};
/** The menu without a session: it asks the reader to sign in. */
export const MenuSignedOut: Story = { parameters: screen({ at: menu, signedOut: true }) };
/** Display and posting, as a new account has it: the language and the time zone follow the browser. */
export const Display: Story = { parameters: screen({ at: display }) };
/**
 * Display and posting with the account's own choices: a time zone, small link previews, posts to the followers, in
 * Japanese, that the followers may reply to at once and others with approval, anyone may boost and the followers may
 * quote, and timelines of two languages that hold new posts back. The language still
 * follows the browser, so that the toolbar's locale shows the screen in each.
 */
export const DisplayChosen: Story = {
  parameters: screen({
    at: display,
    server: {
      preferences: {
        time_zone: "Asia/Tokyo",
        time_zone_chosen: true,
        time_zone_follows_browser: false,
        link_card_size: "small",
        default_visibility: "private",
        default_interaction_policy: {
          reply: { automatic: "followers", manual: "everyone" },
          announce: { automatic: "everyone" },
          quote: { automatic: "followers" },
        },
        default_post_language: "ja",
        require_media_description: true,
        timeline_languages: ["en", "ja"],
        timeline_auto_insert: false,
      },
    },
  }),
};
/** Display and posting on a server that requires media descriptions itself: the switch stands on and says why. */
export const DisplayServerRequires: Story = {
  parameters: screen({ at: display, server: { requiresDescriptions: true } }),
};
/** Display and posting on a server that does not save: a control changed says so, with Try again, and goes back. */
export const DisplaySaveFails: Story = { parameters: screen({ at: display, server: { preferencesFail: "save" } }) };
/** Display and posting when the reader's preferences could not be read: it says so, with the way to try again. */
export const DisplayFailed: Story = { parameters: screen({ at: display, server: { preferencesFail: "read" } }) };
/** The confirmation of signing out, over the menu. */
export const SigningOut: Story = { parameters: screen({ at: `${menu}?sign_out=true` }) };
/** An address under /settings that names no entry. */
export const Unknown: Story = { parameters: screen({ at: "/settings/nothing" }) };
