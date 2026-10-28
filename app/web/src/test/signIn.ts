// SPDX-License-Identifier: AGPL-3.0-only
// Signing in from a test of the whole app: /login is no page, it sends the browser to the server's authorization
// screen at once (routes/login.tsx), which jsdom cannot load; renderApp's `leftTo` records where the app went.
import { waitFor } from "@testing-library/react";
import { expect } from "vitest";
import { signInKey } from "../lib/storage";

/** Waits until the app has left for the server's sign-in screen, to come back to `returnTo` after signing in. */
export async function expectSignInStarted(leftTo: readonly URL[], returnTo: string): Promise<void> {
  await waitFor(() => {
    expect(leftTo.map((url) => url.pathname)).toEqual(["/oauth/authorize"]);
  });
  expect(signInKey.read()?.returnTo).toBe(returnTo);
}
