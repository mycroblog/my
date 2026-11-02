// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { z } from "zod";
import { AnnouncementPageSkeleton, AnnouncementPageView, announcementQueryOptions } from "../features/announcements";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** The announcement's ID (api/spec AnnouncementId). */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * One of the server's announcements, named after its title (FR-726, web-client/screens/announcement.md), which its
 * notification, its Web Push, and the titles of the list and the third column link. It is for the users of this server,
 * so it needs a session. The `_` keeps the page out of the list's layout at `/announcements`.
 */
export const Route = createFileRoute("/announcements_/$id")({
  staticData: { title: "client:announcements.title", back: true, backTo: "/announcements" },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, params: { id } }) => {
    // A failure does not fail the page: the page tells a withdrawn or unknown announcement from the 404, and says why
    // otherwise in its place.
    await queryClient.query({ ...announcementQueryOptions(api, id), staleTime: Infinity }).catch(() => undefined);
  },
  component: AnnouncementPage,
  pendingComponent: AnnouncementPending,
});

function renderHeading(text: ReactNode, id?: string) {
  return <PageHeading {...(id !== undefined && { id })}>{text}</PageHeading>;
}

function AnnouncementPage() {
  const { id } = Route.useParams();
  // Another announcement is another page: nothing of the previous one carries over (web-client/README.md R10).
  return <AnnouncementPageView key={id} id={id} renderHeading={renderHeading} />;
}

/** The page's heading over the shape of the announcement, while it loads (web-client/16-look-states-and-wording.md G15). */
function AnnouncementPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:announcements.title")}</PageHeading>
      <AnnouncementPageSkeleton />
    </>
  );
}
