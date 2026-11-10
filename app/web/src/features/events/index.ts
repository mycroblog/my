// SPDX-License-Identifier: AGPL-3.0-only
// The events of the local users (ADR-0161): the list of the upcoming events (`/events`), "Coming up" in the third
// column, the events an account organizes on its profile, the page of an event (`/events/{id}`) with the user's
// answer, its places, its attendees, its `.ics` file and its organizers' tools, and its editor (`/events/new`,
// `/events/{id}/edit`). Events need a session: nothing of them shows to a visitor who is not signed in. Routes and other
// features use it through this file only (web-client/README.md A3).
export { eventQueryOptions, eventsQueryOptions } from "./api/events";
export { EventEditView, EventForm } from "./components/EventForm";
export { EventListView, EventRail, ProfileEvents } from "./components/EventList";
export { EventListSkeleton, EventPageSkeleton } from "./components/EventSkeletons";
export { EventView } from "./components/EventView";
