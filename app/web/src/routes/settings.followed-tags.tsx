// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { FollowedTags, followedTagsQueryOptions } from "../features/hashtags";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { SettingsHeading } from "./-layout/SettingsHeading";

/** The hashtags the user follows, whose posts join the home timeline (ADR-0013 §24; web-client/screens/settings-followed-tags.md). */
export const Route = createFileRoute("/settings/followed-tags")({
  staticData: { title: "client:tag.followed_list.title" },
  beforeLoad: requireSignIn,
  loader: ({ context }) => context.queryClient.query({ ...followedTagsQueryOptions(context.api), staleTime: Infinity }),
  component: FollowedTagsPage,
});

function FollowedTagsPage() {
  const t = useT();
  return (
    <>
      <SettingsHeading>{t("client:tag.followed_list.title")}</SettingsHeading>
      <FollowedTags />
    </>
  );
}
