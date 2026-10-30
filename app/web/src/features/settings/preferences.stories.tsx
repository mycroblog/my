// SPDX-License-Identifier: AGPL-3.0-only
// The display, posting, and timeline preferences (/settings/preferences; web-client/screens/settings-preferences.md) in
// the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";

const meta = {
  title: "Screens/Settings/Preferences",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/settings/preferences",
      signedIn: true,
      preferences: {
        time_zone: "Asia/Tokyo",
        default_visibility: "private",
        default_post_language: "en",
        timeline_languages: ["en", "ja"],
        require_media_description: true,
      },
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Preferences a person has chosen; a switch saves as soon as it changes and shows so beside it. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const autoInsert = canvas.getByRole("switch", { name: "Show new posts automatically" });
    await userEvent.click(autoInsert);
    await expect(autoInsert).not.toBeChecked();
    await expect(await canvas.findByText("Saved", { selector: "[data-autosave] span" })).toBeInTheDocument();
  },
};

/** Moving Profile up puts it in the tab bar: the preview of the phones' tab bar shows it at once, before Menu. */
export const NavigationMoved: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    const preview = canvas.getByRole("figure", { name: "Tab bar on phones" });
    const labels = () =>
      within(preview)
        .getAllByRole("listitem")
        .map((item) => item.textContent);
    await expect(labels()).toEqual(["Home", "Local", "Federated", "Notifications", "Search", "Menu"]);
    await userEvent.click(canvas.getByRole("button", { name: "Move Profile up" }));
    await expect(labels()).toEqual(["Home", "Local", "Federated", "Notifications", "Profile", "Menu"]);
  },
};

/** A new account's preferences: no timeline languages, no default language of posts, and the server's time zone. */
export const Empty: Story = {
  parameters: {
    screen: {
      preferences: {
        time_zone: "UTC",
        time_zone_chosen: false,
        default_visibility: "public",
        default_post_language: null,
        timeline_languages: [],
        require_media_description: false,
      },
    },
  },
};

/** The preferences have not arrived yet. */
export const Loading: Story = loadingOn("get", "/preferences");

/** The server failed to answer the preferences. */
export const ServerError: Story = failingOn("get", "/preferences");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
