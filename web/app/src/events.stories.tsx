// SPDX-License-Identifier: AGPL-3.0-only

// The events for the attendee (ADR-0161 §5–§12, §15; FR-941–943, FR-946–948, FR-951, FR-952), on the fake server
// (app.samples.tsx): the list of the upcoming events, empty, loading and failing, and for a visitor; an event the reader
// goes to, approved, in another zone than theirs; one they may go to, not approved, its place hidden; one they are
// invited to and have not answered; one at its capacity; one they co-organize; a date poll; a voice place before the
// start, with its room open, and with a room that could not open; cancelled; ended; one the reader does not see; its
// page loading, failing, with long names, and for a visitor; and the ways in: the profile of an organizer with their
// upcoming events, and the reminders in the settings, loading and failing.

//
// And for the organizer (#3238): the list without Create an event, the form that creates one (and its address for a
// reader who may not), changing an event as its organizer and as a co-organizer (and the address for another reader
// and for a cancelled event), the invitations and the co-organizers (and their addresses for a reader who does not
// choose them), and the voice room that could not open, said to its organizer. The poll's dates' actions and the
// approvals show in Poll and CoOrganized.

import { scenario, type FakeEventPlan } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Events",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/events" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The upcoming events the reader sees, the nearest first; a cancelled one stays where the server lists it. */
export const Events: Story = {};
export const EventsEmpty: Story = { parameters: screen({ at: "/events", state: "empty" }) };
export const EventsLoading: Story = { parameters: screen({ at: "/events", state: "loading" }) };
export const EventsError: Story = { parameters: screen({ at: "/events", state: "error" }) };
/** A visitor: the list asks them to sign in. */
export const EventsVisitor: Story = { parameters: screen({ at: "/events", signedOut: true }) };

/**
 * An event the reader goes to, approved: its time in the reader's zone and in Paris, its places shown (a link that
 * leaves the site says where), Going chosen, the calendar file, its organizer and who is going and maybe.
 */
export const Going: Story = { parameters: screen({ at: "/events/{event:book-swap}" }) };
/** One the reader may go to, not approved: the place in person shows its kind alone, and that. */
export const MaybeNotApproved: Story = { parameters: screen({ at: "/events/{event:cooking-night}" }) };
/** An event the reader is invited to and has not answered: the line under the answers says answering shows their name. */
export const Invited: Story = { parameters: screen({ at: "/events/{event:garden-day}" }) };
/** At its capacity: Going cannot be chosen, and the line under the answers says why. */
export const Full: Story = { parameters: screen({ at: "/events/{event:tiny-concert}" }) };
/** One the reader co-organizes, for the organizer's followers, with a capacity: every place shows, its organizers. */
export const CoOrganized: Story = { parameters: screen({ at: "/events/{event:bread-class}" }) };
/**
 * A date poll the reader organizes: no answer to the event and no calendar file, the dates' table with the reader's
 * answers, and who answered each date, folded.
 */
export const Poll: Story = { parameters: screen({ at: "/events/{event:photo-walk}" }) };
/** A voice place before the start: its room opens then. */
export const VoiceBefore: Story = { parameters: screen({ at: "/events/{event:radio-hour}" }) };
/** A voice place whose room is open: Join the room, and the answers closed since the start. */
export const VoiceOpen: Story = { parameters: screen({ at: "/events/{event:coffee-voice}" }) };
/** A voice place whose room could not open at the start, and why; the event goes on. */
export const VoiceUnavailable: Story = { parameters: screen({ at: "/events/{event:trail-talk}" }) };
/** A cancelled event: who cancelled it and the organizer's note, the answers closed, no calendar file. */
export const Cancelled: Story = { parameters: screen({ at: "/events/{event:flea-market}" }) };
/** An event that ended. */
export const Ended: Story = { parameters: screen({ at: "/events/{event:picnic}" }) };
/** An event the reader does not see: the page says so alone. */
export const NotSeen: Story = { parameters: screen({ at: "/events/{event:followers-brunch}" }) };
export const EventLoading: Story = { parameters: screen({ at: "/events/{event:book-swap}", state: "loading" }) };
export const EventError: Story = { parameters: screen({ at: "/events/{event:book-swap}", state: "error" }) };
/** Every name runs long. */
export const EventLong: Story = { parameters: screen({ at: "/events/{event:bread-class}", state: "long" }) };
/** A visitor: only this server's people see events, and Sign in; nothing of the event. */
export const EventVisitor: Story = { parameters: screen({ at: "/events/{event:book-swap}", signedOut: true }) };

/** The profile of an organizer: their upcoming events the reader sees, under the head. */
export const ProfileEvents: Story = { parameters: screen({ at: "/@sam" }) };

/** The reminders in the settings: the defaults, one day and one hour before, and the third none. */
export const Reminders: Story = { parameters: screen({ at: "/settings/events" }) };
export const RemindersLoading: Story = { parameters: screen({ at: "/settings/events", state: "loading" }) };
export const RemindersError: Story = { parameters: screen({ at: "/settings/events", state: "error" }) };

// ---- The organizer's side (ADR-0161 §3–§8, §10; FR-940–945, FR-948, FR-950, FR-952) --------------------------------

/** An event the reader organizes whose voice room could not open: they may not host rooms on this server. */
const unopened: FakeEventPlan = {
  key: "own-voice",
  organizer: "yuki",
  title: "Morning stretch",
  description: "Ten minutes, cameras off.",
  timeZone: "Asia/Tokyo",
  inDays: 0,
  hour: 0,
  hours: 1,
  startsInMinutes: -20,
  places: [{ kind: "voice", speakers: "everyone" }],
  room: "not_eligible",
};

/** An event the reader organized and cancelled. */
const cancelledOwn: FakeEventPlan = {
  key: "own-cancelled",
  organizer: "yuki",
  title: "Kite day",
  description: "On the dunes.",
  timeZone: "Asia/Tokyo",
  inDays: 6,
  hour: 10,
  hours: 3,
  cancelled: { reason: "organizer", note: "Too much wind." },
};

/** The scenario with the reader's own events of these stories beside the fake's. */
const ownEvents = { seed: { ...scenario, events: [...scenario.events, unopened, cancelledOwn] } };

/** The events of a server where only some may create events: no Create an event for this reader. */
export const EventsCannotCreate: Story = {
  parameters: screen({ at: "/events", server: { eventCreators: "permitted" } }),
};
/**
 * The form that creates an event: its title, description, a fixed date tomorrow in the reader's zone, the time zone,
 * the places (the voice room offered to a reader who may host rooms), who sees the event, its capacity.
 */
export const CreateEvent: Story = { parameters: screen({ at: "/events/new" }) };
/** The form's address for a reader who may not create events. */
export const CreateEventNotAllowed: Story = {
  parameters: screen({ at: "/events/new", server: { eventCreators: "permitted" } }),
};
/** Changing an event the reader organizes whose date is polled: its settings, the poll left to its page. */
export const EditEvent: Story = { parameters: screen({ at: "/events/{event:photo-walk}/edit" }) };
/** Changing an event the reader co-organizes: its places, and who sees it said, the organizer's to change. */
export const EditCoOrganized: Story = { parameters: screen({ at: "/events/{event:bread-class}/edit" }) };
/** The form's address for a reader who does not organize the event. */
export const EditNotOrganizer: Story = { parameters: screen({ at: "/events/{event:book-swap}/edit" }) };
/** The form's address of an event the reader cancelled. */
export const EditCancelled: Story = {
  parameters: screen({ at: "/events/{event:own-cancelled}/edit", server: ownEvents }),
};
/** The invitations of an event the reader co-organizes: one, with Revoke, and the search of who to invite. */
export const Invitations: Story = { parameters: screen({ at: "/events/{event:bread-class}/invitations" }) };
/** None yet. */
export const InvitationsNone: Story = { parameters: screen({ at: "/events/{event:photo-walk}/invitations" }) };
/** The invitations' address of an event the reader does not organize. */
export const InvitationsNotOrganizer: Story = {
  parameters: screen({ at: "/events/{event:book-swap}/invitations" }),
};
/** The co-organizers of an event the reader organizes: one, with Remove, and the search of who to name. */
export const Coorganizers: Story = { parameters: screen({ at: "/events/{event:photo-walk}/coorganizers" }) };
/** The co-organizers' address for a co-organizer: only the organizer names them. */
export const CoorganizersNotOrganizer: Story = {
  parameters: screen({ at: "/events/{event:bread-class}/coorganizers" }),
};
/** An event the reader organizes whose voice room could not open, said to them with why. */
export const VoiceUnavailableOrganizer: Story = {
  parameters: screen({
    at: "/events/{event:own-voice}",
    server: ownEvents,
  }),
};
