// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute, type ErrorComponentProps } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { z } from "zod";
import { EventPageSkeleton, EventView, eventQueryOptions } from "../features/events";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";
import { RouteError } from "./-layout/RouteStates";

/** The event's ID (api/spec EventId). */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * The page of an event (ADR-0161 §9, web-client/screens/event.md), which its notifications and the lists link. It
 * needs no session: a visitor who is not signed in learns that only the users of this server take part, with the way
 * to sign in.
 */
export const Route = createFileRoute("/events_/$id")({
  staticData: { reloadSafe: true, back: true, backTo: "/events" },
  params: { parse: (params) => paramsSchema.parse(params) },
  loader: async ({ context: { queryClient, api, session }, params: { id } }) => {
    await session.restore();
    // A failure does not fail the page: the page tells a hidden or unknown event from the 404, and says why otherwise.
    if (session.status === "signed-in") {
      await queryClient.query({ ...eventQueryOptions(api, id), staleTime: Infinity }).catch(() => undefined);
    }
  },
  component: EventPage,
  pendingComponent: EventPending,
  errorComponent: EventError,
});

function renderHeading(text: ReactNode) {
  return <PageHeading>{text}</PageHeading>;
}

function EventPage() {
  const { id } = Route.useParams();
  // Another event is another page: nothing of the previous one carries over (web-client/README.md R10).
  return <EventView key={id} id={id} renderHeading={renderHeading} />;
}

/** The page's heading over the shape of the event, while it loads (web-client/16-look-states-and-wording.md G15). */
function EventPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:events.page.title")}</PageHeading>
      <EventPageSkeleton />
    </>
  );
}

/** The page keeps a heading when it fails (web-client/16-look-states-and-wording.md G16). */
function EventError(props: ErrorComponentProps) {
  const t = useT();
  return <RouteError {...props} heading={t("client:events.page.title")} />;
}
