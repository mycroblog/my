// SPDX-License-Identifier: AGPL-3.0-only
// The guard of the pages that only a signed-in user can use, for their routes' beforeLoad (TanStack Router,
// "Authenticated Routes"). The API checks every request itself; this only sends the browser to sign in first.
import { type ParsedLocation, redirect } from "@tanstack/react-router";
import type { RouterContext } from "../query";

/** Waits for the session to resume; unless signed in, redirects to the sign-in page, which returns to `location`. */
export async function requireSignIn({
  context,
  location,
}: {
  context: RouterContext;
  location: ParsedLocation;
}): Promise<void> {
  await context.session.restore();
  if (context.session.status === "signed-in") return;
  // eslint-disable-next-line typescript/only-throw-error -- TanStack Router's redirects are thrown objects.
  throw redirect({ to: "/login", search: { return_to: location.href } });
}
