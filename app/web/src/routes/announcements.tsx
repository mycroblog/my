// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import {
  AnnouncementHistory,
  AnnouncementHistorySkeleton,
  announcementHistoryQueryOptions,
} from "../features/announcements";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** `id`: the announcement the page is opened for (from its notification, its Web Push, or a title of the panel). */
const searchSchema = z.object({ id: z.string().min(1).optional().catch(undefined) });

/**
 * The server's announcements, those that show now and the ended ones, the latest first (FR-726, ADR-0125 §3,
 * web-client/screens/announcements.md). They are for the users of this server, so it needs a session.
 */
export const Route = createFileRoute("/announcements")({
  staticData: { title: "client:announcements.title" },
  validateSearch: searchSchema,
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api } }) => {
    // A failure does not fail the page: the page shows it in place of the list.
    await queryClient
      .infiniteQuery({ ...announcementHistoryQueryOptions(api), staleTime: Infinity })
      .catch(() => undefined);
  },
  component: AnnouncementsPage,
  pendingComponent: AnnouncementsPending,
});

function AnnouncementsPage() {
  const t = useT();
  const { id } = Route.useSearch();
  return (
    <>
      <PageHeading>{t("client:announcements.title")}</PageHeading>
      <AnnouncementHistory targetId={id} />
    </>
  );
}

/** The page's heading over the shape of the list, while the first page loads (web-client/16-look-states-and-wording.md G15). */
function AnnouncementsPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:announcements.title")}</PageHeading>
      <AnnouncementHistorySkeleton />
    </>
  );
}
