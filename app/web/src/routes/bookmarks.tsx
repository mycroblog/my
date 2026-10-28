// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { useId } from "react";
import { TimelineFeed, TimelineSkeleton, timelineQueryOptions } from "../features/timeline";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** The posts the signed-in user bookmarked, newest bookmark first (FR-113). Bookmarks are private, so it needs a session. */
export const Route = createFileRoute("/bookmarks")({
  staticData: { title: "client:bookmarks.title", hideHeading: true },
  beforeLoad: requireSignIn,
  // Cached pages that no bookmark has marked stale are shown at once (and refetched in the background once old);
  // marked or missing ones are fetched and waited for (web-client/README.md S7).
  loader: ({ context: { queryClient, api } }) =>
    queryClient.infiniteQuery({ ...timelineQueryOptions(api, { kind: "bookmarks" }), staleTime: Infinity }),
  component: BookmarksPage,
  pendingComponent: BookmarksPending,
});

function BookmarksPage() {
  const t = useT();
  const headingId = useId();
  return (
    <>
      <PageHeading id={headingId}>{t("client:bookmarks.title")}</PageHeading>
      <TimelineFeed timeline={{ kind: "bookmarks" }} aria-labelledby={headingId} />
    </>
  );
}

function BookmarksPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:bookmarks.title")}</PageHeading>
      <TimelineSkeleton />
    </>
  );
}
