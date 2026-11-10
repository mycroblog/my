// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { useId } from "react";
import { conversationsQueryOptions, DirectMessagesSkeleton, DirectMessagesView } from "../features/direct-messages";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** The signed-in user's direct messages by conversation (ADR-0154 §6). */
export const Route = createFileRoute("/direct-messages")({
  staticData: { reloadSafe: true, title: "client:direct_messages.title", hideHeading: true },
  // Direct messages are the user's own, so the page needs a session (web-client/10-api-client-and-auth.md §10).
  beforeLoad: requireSignIn,
  // The first page loads with the page. A failure (offline, say) is the list's own, shown below the page's heading
  // with what to do (useDirectMessages), so the loader lets the page open.
  loader: async ({ context: { queryClient, api } }) => {
    await Promise.allSettled([queryClient.infiniteQuery({ ...conversationsQueryOptions(api), staleTime: Infinity })]);
  },
  component: DirectMessagesPage,
  pendingComponent: DirectMessagesPending,
});

function DirectMessagesPage() {
  const t = useT();
  const headingId = useId();
  return (
    <>
      <PageHeading id={headingId}>{t("client:direct_messages.title")}</PageHeading>
      <DirectMessagesView headingId={headingId} />
    </>
  );
}

function DirectMessagesPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:direct_messages.title")}</PageHeading>
      <DirectMessagesSkeleton />
    </>
  );
}
