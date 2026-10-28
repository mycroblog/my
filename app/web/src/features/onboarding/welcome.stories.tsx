// SPDX-License-Identifier: AGPL-3.0-only
// The welcome step after sign-up (/welcome; web-client/screens/welcome.md) in the states of ADR-0112 §1. The first run
// of the home timeline, which comes after it, is with the home timeline's stories.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { inLocale, loadingOn, narrow, renderScreen, screenLoader, settled, viewerWith } from "../../test/stories";

const meta = {
  title: "Screens/Profile/Welcome",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/welcome", signedIn: true, handlers: [viewerWith({ display_name: "" })] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A new user without a display name; Later goes on to the home timeline. */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.type(canvas.getByRole("textbox", { name: /Display name/u }), "Alice Liddell");
    await expect(canvas.getByRole("textbox", { name: /Display name/u })).toHaveValue("Alice Liddell");
  },
};

/** A user who wrote a long display name and profile text at sign-up. */
export const Filled: Story = {
  parameters: {
    screen: {
      handlers: [
        viewerWith({
          display_name: "Maximiliana Theodora Wolfharte",
          note: "<p>Baker, reader, and a very slow runner.</p>",
          source: { note: "Baker, reader, and a very slow runner.", fields: [] },
        }),
      ],
    },
  },
};

/** The account has not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/verify_credentials");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
