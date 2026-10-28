// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { ProfileView } from "../features/profile";
import { pinnedStatusesQueryOptions } from "../features/status";
import { type AccountTimelineFilter, accountTimelineFilters, timelineQueryOptions } from "../features/timeline";
import { PageHeading } from "./-layout/PageHeading";
import { loadAccount, profileParamsSchema } from "./-profile/loadAccount";
import { ProfileError } from "./-profile/ProfileError";
import { ProfileNotFound } from "./-profile/ProfileNotFound";

/** `tab`: the posts the profile lists; omitted, or anything unknown, is its posts without replies (web-client/README.md S3). */
const searchSchema = z.object({
  tab: z.enum(accountTimelineFilters).exclude(["posts"]).optional().catch(undefined),
});

/** The profile of an account (FR-204), at the address the server's HTML shell gives it (ADR-0028). */
export const Route = createFileRoute("/@{$acct}")({
  // A feed to add to: the shell floats the new-post button over this page (web-client/16-look-states-and-wording.md G4).
  staticData: { composeButton: true },
  params: { parse: (params) => profileParamsSchema.parse(params) },
  validateSearch: searchSchema,
  loaderDeps: ({ search: { tab } }): { tab: AccountTimelineFilter } => ({ tab: tab ?? "posts" }),
  // The account and the first page of the selected tab load with the page; cached ones that no write has
  // marked stale are shown at once, marked ones are fetched again first (web-client/README.md S7).
  loader: async ({ context, params: { acct }, deps: { tab } }) => {
    const account = await loadAccount(context, acct);
    await Promise.all([
      context.queryClient.infiniteQuery({
        ...timelineQueryOptions(context.api, { kind: "account", id: account.id, filter: tab }),
        staleTime: Infinity,
      }),
      // The posts tab starts with the account's pinned posts (FR-403).
      tab === "posts" &&
        context.queryClient.query({ ...pinnedStatusesQueryOptions(context.api, account.id), staleTime: Infinity }),
    ]);
  },
  component: ProfilePage,
  notFoundComponent: ProfileNotFound,
  errorComponent: ProfileError,
});

function ProfilePage() {
  const { acct } = Route.useParams();
  const { tab } = Route.useLoaderDeps();
  const navigate = Route.useNavigate();
  return (
    <ProfileView
      // Another account is another page: nothing of the previous one carries over (web-client/README.md R10).
      key={acct}
      acct={acct}
      tab={tab}
      onTabChange={(next) => {
        void navigate({ search: next === "posts" ? {} : { tab: next } });
      }}
      renderHeading={(name) => <PageHeading>{name}</PageHeading>}
    />
  );
}
