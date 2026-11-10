// SPDX-License-Identifier: AGPL-3.0-only
import { createFileRoute } from "@tanstack/react-router";
import { EventForm } from "../features/events";
import { requireSignIn } from "../lib/auth/requireSignIn";
import { useT } from "../lib/i18n";
import { PageHeading } from "./-layout/PageHeading";

/**
 * The editor of a new event (ADR-0161 §4–§6, web-client/screens/event-editor.md), opened from the list of events. The
 * `_` keeps the page out of the list's layout at `/events`.
 */
export const Route = createFileRoute("/events_/new")({
  staticData: { title: "client:events.form.new_title", back: true, backTo: "/events" },
  beforeLoad: requireSignIn,
  component: NewEventPage,
});

function NewEventPage() {
  const t = useT();
  return (
    <>
      <PageHeading>{t("client:events.form.new_title")}</PageHeading>
      <EventForm />
    </>
  );
}
