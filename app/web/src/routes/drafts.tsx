// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { Drafts, draftListsQueryOptions } from "../features/drafts";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The drafts the user saved on the server and their scheduled posts (ADR-0156), from the new-post form's link and the
 * notification of a scheduled post that failed. Drafts are the user's own, so it needs a session.
 */
export const Route = createFileRoute("/drafts")({
  staticData: { reloadSafe: true, title: "client:drafts.title", back: true },
  beforeLoad: requireSignIn,
  loader: ({ context: { queryClient, api } }) =>
    Promise.all(
      draftListsQueryOptions(api).map((options) => queryClient.infiniteQuery({ ...options, staleTime: Infinity })),
    ),
  component: DraftsPage,
});

function DraftsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:drafts.title")}</PageHeading>
      <Drafts />
    </>
  );
}
