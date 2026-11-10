// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { EventListSkeleton, EventListView, eventsQueryOptions } from "../features/events";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The upcoming events that the user sees, the nearest start first, and creating one (ADR-0161 §9,
 * web-client/screens/events.md). Events are for the users of this server, so it needs a session.
 */
export const Route = createFileRoute("/events")({
  staticData: { title: "client:events.title" },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api } }) => {
    // A failure does not fail the page: the page shows it in its body.
    await queryClient.infiniteQuery({ ...eventsQueryOptions(api), staleTime: Infinity }).catch(() => undefined);
  },
  component: EventsPage,
  pendingComponent: EventsPending,
});

function EventsPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:events.title")}</PageHeading>
      <EventListView />
    </>
  );
}

/** The page's heading over the shape of the events, while the first page loads (web-client/16-look-states-and-wording.md G15). */
function EventsPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:events.title")}</PageHeading>
      <EventListSkeleton />
    </>
  );
}
