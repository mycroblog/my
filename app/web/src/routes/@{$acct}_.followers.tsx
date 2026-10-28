// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { followListQueryOptions, FollowListSkeleton, FollowListView } from "../features/profile";
import { PageHeading } from "./-layout/PageHeading";
import { loadAccount, profileParamsSchema } from "./-profile/loadAccount";
import { FollowListError } from "./-profile/ProfileError";
import { ProfileNotFound } from "./-profile/ProfileNotFound";

/** The accounts that follow an account (FR-301). */
export const Route = createFileRoute("/@{$acct}_/followers")({
  staticData: { back: true },
  params: { parse: (params) => profileParamsSchema.parse(params) },
  loader: async ({ context, params: { acct } }) => {
    const account = await loadAccount(context, acct);
    await context.queryClient.infiniteQuery({
      ...followListQueryOptions(context.api, account.id, "followers"),
      staleTime: Infinity,
    });
  },
  component: FollowersPage,
  pendingComponent: FollowersPending,
  notFoundComponent: ProfileNotFound,
  errorComponent: FollowersError,
});

function FollowersPage() {
  const { acct } = Route.useParams();
  return (
    <FollowListView
      key={acct}
      acct={acct}
      list="followers"
      renderHeading={(text) => <PageHeading>{text}</PageHeading>}
    />
  );
}

function FollowersPending() {
  const { acct } = Route.useParams();
  return (
    <FollowListSkeleton acct={acct} list="followers" renderHeading={(text) => <PageHeading>{text}</PageHeading>} />
  );
}

function FollowersError(props: ErrorComponentProps) {
  return <FollowListError {...props} list="followers" />;
}
