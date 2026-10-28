// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import { followListQueryOptions, FollowListSkeleton, FollowListView } from "../features/profile";
import { PageHeading } from "./-layout/PageHeading";
import { loadAccount, profileParamsSchema } from "./-profile/loadAccount";
import { FollowListError } from "./-profile/ProfileError";
import { ProfileNotFound } from "./-profile/ProfileNotFound";

/** The accounts that an account follows (FR-301). */
export const Route = createFileRoute("/@{$acct}_/following")({
  staticData: { back: true },
  params: { parse: (params) => profileParamsSchema.parse(params) },
  loader: async ({ context, params: { acct } }) => {
    const account = await loadAccount(context, acct);
    await context.queryClient.infiniteQuery({
      ...followListQueryOptions(context.api, account.id, "following"),
      staleTime: Infinity,
    });
  },
  component: FollowingPage,
  pendingComponent: FollowingPending,
  notFoundComponent: ProfileNotFound,
  errorComponent: FollowingError,
});

function FollowingPage() {
  const { acct } = Route.useParams();
  return (
    <FollowListView
      key={acct}
      acct={acct}
      list="following"
      renderHeading={(text) => <PageHeading>{text}</PageHeading>}
    />
  );
}

function FollowingPending() {
  const { acct } = Route.useParams();
  return (
    <FollowListSkeleton acct={acct} list="following" renderHeading={(text) => <PageHeading>{text}</PageHeading>} />
  );
}

function FollowingError(props: ErrorComponentProps) {
  return <FollowListError {...props} list="following" />;
}
