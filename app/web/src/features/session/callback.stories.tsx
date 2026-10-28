// SPDX-License-Identifier: AGPL-3.0-only
// The way back from the server's sign-in screen (/login/callback; ADR-0030): the app checks the state, exchanges the
// code, and opens the page the sign-in started from, or says why the sign-in failed (SignInFailure), in the states of
// ADR-0112 §1. Each story has a state of its own, since the app completes a callback once per state.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import { codeExchangeHandler, oauthPaths } from "../../test/msw/http";
import { inLocale, narrow, renderScreen, screenLoader, settled, type ScreenParameters } from "../../test/stories";

/**
 * The callback of a sign-in with `state` that this tab started to come back to `returnTo`, answered with `response`.
 * The failures show the page that says why (`errorPage`).
 */
function callback(
  state: string,
  response: "code" | "access_denied",
  {
    started = true,
    returnTo = "/search",
    ...screen
  }: Partial<ScreenParameters> & { started?: boolean; returnTo?: string } = {},
) {
  const query = response === "code" ? `code=story-code&state=${state}` : `error=access_denied&state=${state}`;
  return {
    parameters: {
      screen: {
        path: `/login/callback?${query}`,
        errorPage: true,
        ...(started && { pendingSignIn: { state, codeVerifier: "story-code-verifier", returnTo } }),
        ...screen,
      },
    },
  };
}

const meta = {
  title: "Screens/Session/Callback",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/login/callback" } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The server accepted the code: the app is signed in and opens the page the sign-in started from. */
export const Completed: Story = {
  ...callback("story-completed", "code", { errorPage: false, handlers: [codeExchangeHandler] }),
  play: async (context) => {
    const { canvas } = context;
    await settled(context);
    await expect(canvas.getByRole("heading", { level: 1, name: "Search" })).toBeInTheDocument();
    await expect(canvas.queryByRole("link", { name: "Sign in" })).not.toBeInTheDocument();
  },
};

/** The person declined on the server's sign-in screen. */
export const Denied: Story = {
  ...callback("story-denied", "access_denied"),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("The sign-in was cancelled on the server's screen.")).toBeInTheDocument();
    await expect(context.canvas.getByRole("link", { name: "Sign in again" })).toHaveAttribute("href", "/login");
  },
};

/** A callback for a sign-in that this tab did not start, or that it completed already. */
export const NotStartedHere: Story = {
  ...callback("story-not-started", "code", { started: false }),
  play: async (context) => {
    await settled(context);
    await expect(
      context.canvas.getByText(
        "This sign-in was not started in this tab, or it has already been completed. Start again.",
      ),
    ).toBeInTheDocument();
  },
};

/** The server refused the code (it expired, say). */
export const Rejected: Story = {
  ...callback("story-rejected", "code"),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("The server did not accept the sign-in. Start again.")).toBeInTheDocument();
  },
};

/** The server could not be reached to exchange the code. */
export const NetworkError: Story = callback("story-network-error", "code", {
  handlers: [http.post(oauthPaths.token, () => HttpResponse.error())],
});

/** A failed sign-in in Japanese. */
export const Japanese: Story = { ...callback("story-ja", "access_denied"), globals: inLocale("ja").globals };

/** A failed sign-in right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = { ...callback("story-ar-xb", "access_denied"), globals: inLocale("ar-XB").globals };

/** A failed sign-in at 320 CSS px. */
export const Narrow: Story = { ...callback("story-narrow", "access_denied"), ...narrow };
