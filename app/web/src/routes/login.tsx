// SPDX-License-Identifier: AGPL-3.0-only
import { Spinner } from "@my/ui";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { z } from "zod";
import { useSignIn } from "../features/session";
import { isReturnPath } from "../lib/auth/signIn";
import { useT } from "../lib/i18n";

/** `return_to`: the path of this app to open after signing in. Anything else is dropped (web-client/README.md S3). */
const searchSchema = z.object({
  return_to: z.string().refine(isReturnPath).optional().catch(undefined),
});

/**
 * `/login` is not a page: it starts signing in at once, sending the browser to the server's sign-in screen (ADR-0030)
 * with a new PKCE challenge, and the callback returns to `return_to`. Every sign-in link of the app (the top bar, the
 * pages that ask to sign in, the pages that need a session) leads here, and so do bookmarks of it.
 */
export const Route = createFileRoute("/login")({
  staticData: { title: "client:login.title" },
  validateSearch: searchSchema,
  component: StartSignIn,
});

function StartSignIn() {
  const t = useT();
  const { return_to: returnTo = "/" } = Route.useSearch();
  const { signIn } = useSignIn(returnTo);
  // Once per visit: a second start (React's development double effects) would replace the first one's state.
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    signIn();
  }, [signIn]);
  return <Spinner label={t("client:login.title")} />;
}
