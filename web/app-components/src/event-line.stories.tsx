// SPDX-License-Identifier: AGPL-3.0-only

// An event as a line of a list (ADR-0161 §9, FR-951), from the event the API sends: a date fixed in another zone than
// the reader's, with every kind of place, one of them hidden from the reader; a date polled; cancelled; and ended. The
// names are samples.

import { RowItem, RowList } from "@my/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { EventListLine, type EventShown } from "./event-line";
import type { PostAccount } from "./status";

const account = (username: string, display_name: string): PostAccount => ({
  id: username,
  username,
  acct: username,
  domain: "example.social",
  display_name,
  emojis: [],
});

const hour = 3_600_000;
const day = 24 * hour;
// The stories' moment, fixed so that a shot shows the same days.
const now = Date.parse("2026-11-19T03:00:00Z");
const at = (ms: number) => new Date(now + ms).toISOString();

const event = (over: Partial<EventShown> & Pick<EventShown, "id" | "title">): EventShown => ({
  organizer: account("mika", "Mika"),
  time_zone: "Asia/Tokyo",
  starts_at: at(2 * day),
  ends_at: at(2 * day + 2 * hour),
  state: "scheduled",
  places: [],
  hidden_places: [],
  going_count: 0,
  ...over,
});

const events: readonly EventShown[] = [
  event({
    id: "1",
    title: "Book club: The Left Hand of Darkness",
    time_zone: "Europe/Paris",
    places: [{ kind: "voice" }, { kind: "url" }],
    hidden_places: ["physical"],
    going_count: 12,
  }),
  event({
    id: "2",
    title: "Winter meetup",
    state: "unscheduled",
    hidden_places: ["physical"],
    organizer: account("sora", "Sora"),
  }),
  event({ id: "3", title: "Morning walk", state: "cancelled", going_count: 4 }),
  event({
    id: "4",
    title: "Sketch night",
    starts_at: at(-3 * hour),
    ends_at: at(-hour),
    places: [{ kind: "physical" }],
    going_count: 7,
  }),
];

const meta = { title: "Event line", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Events of a list: a date fixed, its own zone's time beside the reader's; a date polled; cancelled; ended. */
export const Lines: Story = {
  render: () => (
    <RowList label="Upcoming events">
      {events.map((e) => (
        <RowItem key={e.id}>
          <EventListLine event={e} now={now} />
        </RowItem>
      ))}
    </RowList>
  ),
};
