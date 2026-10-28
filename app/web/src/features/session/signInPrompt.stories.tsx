// SPDX-License-Identifier: AGPL-3.0-only
// The way to sign in that a page needing a session shows in its place (SignInPrompt; web-client/10-api-client-and-auth.md
// §10), in the states of ADR-0112 §1: the home timeline opened while the server cannot be reached to resume the session,
// with no posts kept on this browser (web-client/11-pwa.md §11). The pages that need a session send a browser that is
// signed out to sign in instead (signIn.stories.tsx).
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import { oauthPaths } from "../../test/msw/http";
import { inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";

/** The refresh endpoint cannot be reached: the session can neither resume nor be known to be gone. */
const unreachable = http.post(oauthPaths.refresh, () => HttpResponse.error());

const meta = {
  title: "Screens/Session/SignInPrompt",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/home", handlers: [unreachable] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The home timeline says that nobody is signed in, and its link signs in and comes back here. */
export const Home: Story = {
  play: async (context) => {
    const { canvas } = context;
    await settled(context);
    await expect(canvas.getByRole("heading", { name: "You are not signed in" })).toBeInTheDocument();
    await expect(canvas.getByRole("link", { name: "Sign in" })).toHaveAttribute(
      "href",
      expect.stringContaining("/login?return_to=%2Fhome"),
    );
  },
};

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
