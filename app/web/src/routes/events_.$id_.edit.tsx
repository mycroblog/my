// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { z } from "zod";
import { EventEditView, EventPageSkeleton, eventQueryOptions } from "../features/events";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/** The event's ID (api/spec EventId). */
const paramsSchema = z.object({ id: z.string().min(1).max(64) });

/**
 * The editor of an event that the user organizes or co-organizes (ADR-0161 §4, web-client/screens/event-editor.md),
 * opened from the event's page. The `_`s keep it out of the layouts of the list and of the event's page.
 */
export const Route = createFileRoute("/events_/$id_/edit")({
  staticData: { title: "client:events.form.edit_title", back: true },
  params: { parse: (params) => paramsSchema.parse(params) },
  beforeLoad: requireSignIn,
  loader: async ({ context: { queryClient, api }, params: { id } }) => {
    // A failure does not fail the page: the editor says that the event is not there, or why it failed.
    await queryClient.query({ ...eventQueryOptions(api, id), staleTime: Infinity }).catch(() => undefined);
  },
  component: EditEventPage,
  pendingComponent: EditEventPending,
});

function renderHeading(text: string) {
  return <PageHeading>{text}</PageHeading>;
}

function EditEventPage() {
  const { id } = Route.useParams();
  // Another event is another draft: nothing of the previous one carries over (web-client/README.md R10).
  return <EventEditView key={id} id={id} renderHeading={renderHeading} />;
}

function EditEventPending() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:events.form.edit_title")}</PageHeading>
      <EventPageSkeleton />
    </>
  );
}
