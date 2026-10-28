// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { FollowRequests, followRequestsQueryOptions } from "../features/settings";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The follow requests that await the user's approval (FR-302). */
export const Route = createFileRoute("/settings/follow-requests")({
  staticData: { title: "client:settings.follow_requests.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) =>
    context.queryClient.infiniteQuery({ ...followRequestsQueryOptions(context.api), staleTime: Infinity }),
  component: FollowRequestsPage,
});

function FollowRequestsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:settings.follow_requests.title")}</SettingsHeading>
      <FollowRequests />
    </>
  );
}
