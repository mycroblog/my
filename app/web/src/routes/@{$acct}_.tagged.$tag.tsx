// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { z } from "zod";
import { isolate, TaggedPostsSkeleton, TaggedPostsView } from "../features/profile";
import { timelineQueryOptions } from "../features/timeline";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";
import { loadAccount, profileParamsSchema } from "./-profile/loadAccount";
import { ProfileNotFound } from "./-profile/ProfileNotFound";

/** The hashtag, without the `#` (the featured hashtags' `href`, ADR-0006 §17). */
const paramsSchema = profileParamsSchema.extend({ tag: z.string().min(1).max(256) });

/**
 * The posts of an account that carry a hashtag (web-client/screens/profile-tagged.md), at the address that a featured
 * hashtag of its profile gives other servers (`/@alice/tagged/cats`, ADR-0006 §17).
 */
export const Route = createFileRoute("/@{$acct}_/tagged/$tag")({
  // A feed to add to: the shell floats the new-post button over this page (web-client/16-look-states-and-wording.md G4).
  staticData: { reloadSafe: true, composeButton: true, back: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  // Cached pages that no write has marked stale are shown at once; marked or missing ones are fetched and waited for
  // (web-client/README.md S7).
  loader: async ({ context, params: { acct, tag } }) => {
    const account = await loadAccount(context, acct);
    await context.queryClient.infiniteQuery({
      ...timelineQueryOptions(context.api, { kind: "tagged", id: account.id, tag }),
      staleTime: Infinity,
    });
  },
  component: TaggedPage,
  pendingComponent: TaggedPending,
  notFoundComponent: ProfileNotFound,
  errorComponent: TaggedError,
});

function renderHeading(text: ReactNode, id?: string) {
  return <PageHeading {...(id !== undefined && { id })}>{text}</PageHeading>;
}

function TaggedPage() {
  const { acct, tag } = Route.useParams();
  return <TaggedPostsView key={acct} acct={acct} tag={tag} renderHeading={renderHeading} />;
}

function TaggedPending() {
  const { acct, tag } = Route.useParams();
  return <TaggedPostsSkeleton acct={acct} tag={tag} renderHeading={renderHeading} />;
}

/** The page keeps its name, with the handle in place of the account's name, when it fails (web-client/16-look-states-and-wording.md G16). */
function TaggedError(props: ErrorComponentProps) {
  const t = useT();
  const { acct, tag } = Route.useParams();
  return (
    <RouteError
      {...props}
      heading={t("client:profile.tagged.title", { name: isolate(`@${acct}`), tag: isolate(tag) })}
    />
  );
}
