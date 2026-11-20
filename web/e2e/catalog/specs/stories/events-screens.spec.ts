// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the events' screens (web/app/src/events.stories.tsx) each show the state they are named by, on the
// fake API: the list's own states and a visitor's, an event's answer in its states (going, not answered, at capacity,
// started), one co-organized, its places (hidden, a voice room before the start, open, and one that could not open), a
// poll, cancelled, ended, not seen, its loading and failure, a visitor's page, the organizer's profile, and the
// reminders, loading and failing; and the organizer's: the list without Create an event, the form that creates one and
// its address for a reader who may not, changing an event (an organizer's, a co-organizer's, another reader's, a
// cancelled one), the invitations and the co-organizers and their addresses for others, and the voice room that could
// not open said to its organizer. What the screens do from each state is the app's route tests'
// (web/app/src/events-routes.dom.test.tsx, events-organize.dom.test.tsx); this is that the catalog shows each state for
// a review and a shot.

import type { Page } from "@playwright/test";
import { expect, pc, test } from "../test";

/** The story of the events' screens named `name`. */
const story = (name: string) => `screens-events--${name}`;

const main = (page: Page) => page.getByRole("main");

/** The events the list shows, as links to their pages. */
const listed = (page: Page) => main(page).getByRole("list", { name: "Upcoming events", exact: true }).getByRole("link");

/** The reader's answers to an event. */
const answers = (page: Page) => main(page).getByRole("radiogroup", { name: "Your answer", exact: true });

/** The reader's answer `name` to an event. */
const answer = (page: Page, name: string) => answers(page).getByRole("radio", { name, exact: true });

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    "events",
    async (page) => {
      await expect(listed(page)).not.toHaveCount(0);
    },
  ],
  [
    "events-empty",
    async (page) => {
      await expect(main(page).getByText(/^No upcoming events\.$/)).toBeVisible();
      await expect(listed(page)).toHaveCount(0);
    },
  ],
  [
    "events-loading",
    async (page) => {
      await expect(main(page).getByText(/^Loading events…$/)).toBeVisible();
    },
  ],
  [
    "events-error",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^The events could not be loaded\.\s*Try again$/);
    },
  ],
  [
    "events-visitor",
    async (page) => {
      await expect(main(page).getByRole("link", { name: "Sign in", exact: true })).toBeVisible();
      await expect(listed(page)).toHaveCount(0);
    },
  ],
  [
    "going",
    async (page) => {
      await expect(answer(page, "Going")).toBeChecked();
      await expect(main(page).getByRole("button", { name: "Add to calendar", exact: true })).toBeVisible();
      await expect(main(page).getByRole("list", { name: /^Going \(\d+\)$/ })).toBeVisible();
    },
  ],
  [
    "maybe-not-approved",
    async (page) => {
      await expect(answer(page, "Maybe")).toBeChecked();
      await expect(
        main(page).getByText(/^In person: the place is shown to the attendees the organizers approve$/),
      ).toBeVisible();
    },
  ],
  [
    "invited",
    async (page) => {
      // Not answered yet: the answers stand with none chosen, and the lists show whose answer by their headings.
      await expect(answers(page).getByRole("radio", { checked: true })).toHaveCount(0);
      await expect(answer(page, "Going")).toBeEnabled();
    },
  ],
  [
    "full",
    async (page) => {
      await expect(answer(page, "Going")).toBeDisabled();
      await expect(
        main(page).getByText(/^The event is full\. You can still answer maybe or not going\.$/),
      ).toBeVisible();
      // The headcount of the capacity: its figure for the eye, its sentence for assistive technology.
      await expect(main(page).getByText(/^3\/3$/)).toBeVisible();
      await expect(main(page).getByText(/^3 of 3 going$/)).toHaveCount(1);
    },
  ],
  [
    "co-organized",
    async (page) => {
      // A co-organizer sees every place, and is among the organizers.
      await expect(main(page).getByText(/^2-4-1 Harbor Street, flat 3$/)).toBeVisible();
      await expect(
        main(page).getByRole("link", { name: /^The class on video \(opens meet\.example in a new tab\)$/ }),
      ).toBeVisible();
      await expect(main(page).getByRole("list", { name: "Organizers", exact: true }).getByRole("listitem")).toHaveCount(
        2,
      );
      // The organizers' menu, and each attendee's approval.
      await expect(main(page).getByRole("button", { name: "Manage the event", exact: true })).toBeVisible();
      // Two wait for approval, each with Approve in sight; the one approved has its mark and its menu.
      await expect(main(page).getByText(/^Waiting for approval$/)).toHaveCount(2);
      await expect(main(page).getByRole("button", { name: /^Approve .+$/ })).toHaveCount(2);
      await expect(main(page).getByText(/^Approved$/)).toHaveCount(1);
      await expect(main(page).getByRole("button", { name: /^Actions for .+$/ })).toHaveCount(1);
    },
  ],
  [
    "poll",
    async (page) => {
      await expect(main(page).getByRole("table", { name: "Date poll", exact: true })).toBeVisible();
      await expect(answers(page)).toHaveCount(0);
      await expect(main(page).getByRole("button", { name: "Add to calendar", exact: true })).toHaveCount(0);
      // The reader organizes it: each date's menu, and a date to add.
      await expect(main(page).getByRole("button", { name: /^Options for .+$/ })).toHaveCount(3);
      await expect(main(page).getByRole("group", { name: "Add a date", exact: true })).toBeVisible();
    },
  ],
  [
    "voice-before",
    async (page) => {
      await expect(main(page).getByText(/^Opens when the event starts$/)).toBeVisible();
    },
  ],
  [
    "voice-open",
    async (page) => {
      await expect(main(page).getByRole("button", { name: "Join the room", exact: true })).toBeVisible();
      await expect(answer(page, "Going")).toBeDisabled();
    },
  ],
  [
    "voice-unavailable",
    async (page) => {
      await expect(
        main(page).getByText(/^Could not open: as many rooms were open as this server allows$/),
      ).toBeVisible();
    },
  ],
  [
    "cancelled",
    async (page) => {
      await expect(main(page).getByText(/^Cancelled$/)).toBeVisible();
      await expect(main(page).getByText(/^This event was cancelled by its organizer\.$/)).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Add to calendar", exact: true })).toBeVisible();
    },
  ],
  [
    "ended",
    async (page) => {
      await expect(main(page).getByText(/^Ended$/)).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Add to calendar", exact: true })).toHaveCount(0);
    },
  ],
  [
    "not-seen",
    async (page) => {
      await expect(main(page).getByText(/^This event does not exist, or you cannot see it\.$/)).toBeVisible();
    },
  ],
  [
    "event-loading",
    async (page) => {
      await expect(main(page).getByText(/^Loading the event…$/)).toBeVisible();
    },
  ],
  [
    "event-error",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^The event could not be loaded\.\s*Try again$/);
    },
  ],
  [
    "event-visitor",
    async (page) => {
      await expect(main(page).getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
      await expect(answers(page)).toHaveCount(0);
    },
  ],
  [
    "profile-events",
    async (page) => {
      await expect(main(page).getByRole("list", { name: "Upcoming events", exact: true })).toBeVisible();
    },
  ],
  [
    "reminders",
    async (page) => {
      await expect(main(page).getByRole("combobox", { name: "Reminder 1", exact: true })).toBeVisible();
    },
  ],
  [
    "reminders-loading",
    async (page) => {
      await expect(main(page).getByText(/^Loading your reminders…$/)).toBeVisible();
      await expect(main(page).getByRole("combobox")).toHaveCount(0);
    },
  ],
  [
    "reminders-error",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^Your reminders could not be loaded\./);
    },
  ],
  // The organizer's (#3238).
  [
    "events-cannot-create",
    async (page) => {
      await expect(listed(page)).not.toHaveCount(0);
      await expect(page.getByRole("link", { name: "Create an event", exact: true })).toHaveCount(0);
    },
  ],
  [
    "create-event",
    async (page) => {
      await expect(main(page).getByRole("textbox", { name: "Title", exact: true })).toBeVisible();
      await expect(main(page).getByRole("radio", { name: "On a fixed date", exact: true })).toBeChecked();
      await expect(
        main(page).getByRole("checkbox", {
          name: "In a voice chat room that opens when the event starts",
          exact: true,
        }),
      ).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Create the event", exact: true })).toBeVisible();
    },
  ],
  [
    "create-event-not-allowed",
    async (page) => {
      await expect(main(page).getByText(/^You cannot create events on this server\..*$/)).toBeVisible();
      await expect(main(page).getByRole("textbox")).toHaveCount(0);
    },
  ],
  [
    "edit-event",
    async (page) => {
      await expect(main(page).getByRole("textbox", { name: "Title", exact: true })).toHaveValue(/^Photo walk/);
      await expect(main(page).getByText(/^The date is being polled\..*$/)).toBeVisible();
      await expect(main(page).getByRole("radiogroup", { name: "Who can see the event", exact: true })).toBeVisible();
    },
  ],
  [
    "edit-co-organized",
    async (page) => {
      await expect(main(page).getByRole("checkbox", { name: "In person", exact: true })).toBeChecked();
      await expect(main(page).getByText(/^.+ Only the organizer, .+, changes who can see it\.$/)).toBeVisible();
      await expect(main(page).getByRole("radiogroup", { name: "Who can see the event", exact: true })).toHaveCount(0);
    },
  ],
  [
    "edit-not-organizer",
    async (page) => {
      await expect(
        main(page).getByText(/^Only the event's organizer, .+, and its co-organizers can change it\.$/),
      ).toBeVisible();
    },
  ],
  [
    "edit-cancelled",
    async (page) => {
      await expect(main(page).getByText(/^This event was cancelled, so it can no longer be changed\.$/)).toBeVisible();
    },
  ],
  [
    "invitations",
    async (page) => {
      await expect(main(page).getByRole("list", { name: "Invited", exact: true }).getByRole("listitem")).toHaveCount(1);
      await expect(main(page).getByRole("button", { name: /^Actions for .+$/ })).toHaveCount(1);
      await expect(main(page).getByRole("searchbox", { name: "Name or handle", exact: true })).toBeVisible();
    },
  ],
  [
    "invitations-none",
    async (page) => {
      await expect(main(page).getByText(/^No one is invited yet\..*$/)).toBeVisible();
    },
  ],
  [
    "invitations-not-organizer",
    async (page) => {
      await expect(
        main(page).getByText(/^Only the event's organizer, .+, and its co-organizers can invite people to it\.$/),
      ).toBeVisible();
      await expect(main(page).getByRole("searchbox")).toHaveCount(0);
    },
  ],
  [
    "coorganizers",
    async (page) => {
      await expect(main(page).getByRole("button", { name: /^Actions for .+$/ })).toHaveCount(1);
      await expect(main(page).getByRole("searchbox", { name: "Name or handle", exact: true })).toBeVisible();
    },
  ],
  [
    "coorganizers-not-organizer",
    async (page) => {
      await expect(
        main(page).getByText(/^Only the event's organizer, .+, can name its co-organizers\.$/),
      ).toBeVisible();
    },
  ],
  [
    "voice-unavailable-organizer",
    async (page) => {
      await expect(
        main(page).getByText(
          /^Could not open: you may not host voice chat rooms on this server\. The event goes on\.$/,
        ),
      ).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Manage the event", exact: true })).toBeVisible();
    },
  ],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the events shows its state", async ({ page, open }) => {
  for (const [name, shows] of states)
    await test.step(name, async () => {
      await open(story(name), { width: pc });
      await shows(page);
    });
});
