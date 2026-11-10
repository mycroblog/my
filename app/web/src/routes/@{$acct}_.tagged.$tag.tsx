// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, redirect } from "@tanstack/react-router";
import { z } from "zod";
import { profileParamsSchema } from "./-profile/loadAccount";

/** The hashtag, without the `#` (the featured hashtags' `href`, ADR-0006 §17). */
const paramsSchema = profileParamsSchema.extend({ tag: z.string().min(1).max(256) });

/**
 * The address that a featured hashtag of a profile gives other servers (`/@alice/tagged/cats`, ADR-0006 §17). The
 * client API lists no account's posts by hashtag, so it is replaced by the hashtag's page (web-client/screens/profile.md).
 */
export const Route = createFileRoute("/@{$acct}_/tagged/$tag")({
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: ({ params: { tag } }) => {
    // eslint-disable-next-line typescript/only-throw-error -- TanStack Router's redirects are thrown objects.
    throw redirect({ to: "/tags/$tag", params: { tag }, replace: true });
  },
});
