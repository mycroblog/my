// SPDX-License-Identifier: AGPL-3.0-only
// Starting to sign in (/login; ADR-0030): the page is only a spinner while the app remembers a new PKCE verifier and
// state and sends the browser to the server's sign-in screen, which the stories do not leave for. Every sign-in link
// leads here, and so does a page that needs a session, opened signed out.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { waitFor } from "storybook/test";
import { signInKey } from "../../lib/storage";
import { inLocale, narrow, renderScreen, screenLoader } from "../../test/stories";

/** Waits for the spinner, and for the sign-in this tab started to come back to `returnTo`. */
function startsSigningIn(returnTo: string) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await waitFor(
      () => {
        if (canvasElement.querySelector('[role="progressbar"]') === null) throw new Error("no spinner yet");
        if (signInKey.read()?.returnTo !== returnTo) throw new Error(`no sign-in returning to ${returnTo} yet`);
      },
      { timeout: 10_000 },
    );
  };
}

const meta = {
  title: "Screens/Session/SignIn",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/login?return_to=%2Fsettings" } },
  play: startsSigningIn("/settings"),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A sign-in link of the app: the sign-in comes back to the page it was opened from. */
export const Starting: Story = {};

/** A page that needs a session (bookmarks), opened signed out: it sends the browser to sign in and come back. */
export const FromPageNeedingSignIn: Story = {
  parameters: { screen: { path: "/bookmarks" } },
  play: startsSigningIn("/bookmarks"),
};

/** A bookmark of /login itself: the sign-in comes back to the start of the app. */
export const WithoutReturn: Story = {
  parameters: { screen: { path: "/login" } },
  play: startsSigningIn("/"),
};

/** In Japanese. */
export const Japanese: Story = { globals: inLocale("ja").globals };

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = { globals: inLocale("ar-XB").globals };

/** At 320 CSS px. */
export const Narrow: Story = narrow;
