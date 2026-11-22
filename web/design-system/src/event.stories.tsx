// SPDX-License-Identifier: AGPL-3.0-only

// The parts of an event (ADR-0161, FR-951, FR-952), each in every state, and an event's page they make together: its
// time, in the reader's zone alone and over the event's own; events as lines of a list (a date fixed and its own
// zone, a date polled, cancelled, ended, a title that wraps, no place) and in the column beside the main one; the
// reader's answer before answering, answered, saving, saved and not saved, at capacity, before a date is fixed and
// after the start; the date poll's table for a participant, for an organizer with each date's actions, with a date
// confirmed and the poll closed; and the places by kind, the voice room before, during and after it could not open,
// in person with and without its map, online, and hidden from the reader. The names are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { LinkButton } from "./button";
import {
  AnswerChoice,
  CandidateRow,
  CandidateTable,
  EventLine,
  EventTime,
  PlaceLine,
  PlaceList,
  type Answer,
} from "./event";
import { IconButton } from "./icon-button";
import { Box, Row, Stack } from "./layout";
import { GroupHeading, RowItem, RowList, ScreenTitle } from "./list";
import { MenuGroup, MenuItem, MenuSurface } from "./menu";
import { SaveState } from "./save-state";
import { FrameSection, SectionList, SectionLine } from "./shell";
import { HeadcountMark } from "./voice";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

/** The column beside the main one: as wide as the frame's. */
function Aside({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(300), padding: rem(16) }}>{children}</div>;
}

const going = { yes: "Going", maybe: "Maybe", no: "Not going" } as const;
const poll = { yes: "Can go", maybe: "Maybe", no: "Cannot" } as const;

/** Times as the app formats them, in the reader's zone (Los Angeles) and, where it differs, the event's (Tokyo). */
const times = {
  run: (
    <EventTime
      dateTime="2026-11-21T05:00:00Z"
      shown="Fri, Nov 20, 9:00 – 11:00 PM"
      own={{ shown: "Sat 2:00 PM", zone: "Tokyo" }}
    />
  ),
  release: <EventTime dateTime="2026-11-24T03:00:00Z" shown="Mon, Nov 23, 7:00 – 8:30 PM" />,
  meetup: <EventTime dateTime="2026-11-29T01:00:00Z" shown="Sat, Nov 28, 5:00 PM" />,
  past: <EventTime dateTime="2026-11-10T03:00:00Z" shown="Mon, Nov 9, 7:00 PM" />,
};

const allPlaces = [
  { kind: "voice", label: "Voice chat" },
  { kind: "physical", label: "In person" },
  { kind: "url", label: "Online" },
] as const;

/** Events as lines of the events' screen. */
function Lines() {
  return (
    <RowList label="Upcoming events">
      <RowItem>
        <EventLine
          href="/events/1"
          title="Book club: The Left Hand of Darkness"
          date={{ month: "Nov", day: "20" }}
          time={times.run}
          places={allPlaces}
          organizer="By Mika"
          going={{ shown: "12", label: "12 going" }}
        />
      </RowItem>
      <RowItem>
        <EventLine
          href="/events/2"
          title="Release party for v0.2"
          date={{ month: "Nov", day: "23" }}
          time={times.release}
          places={[{ kind: "voice", label: "Voice chat" }]}
          organizer="By Ren"
          going={{ shown: "48", label: "48 going" }}
        />
      </RowItem>
      <RowItem>
        <EventLine
          href="/events/3"
          title="Winter meetup"
          time="Date to be decided · 4 dates proposed"
          places={[{ kind: "physical", label: "In person" }]}
          organizer="By Sora"
        />
      </RowItem>
      <RowItem>
        <EventLine
          href="/events/4"
          title="A meetup with a title long enough to wrap onto a second line, and then to be cut at the end of it"
          date={{ month: "Nov", day: "28" }}
          time={times.meetup}
          places={[{ kind: "url", label: "Online" }]}
          organizer="By someone with a display name that is long too"
          going={{ shown: "3", label: "3 going" }}
        />
      </RowItem>
      <RowItem>
        <EventLine
          href="/events/5"
          title="Morning walk"
          date={{ month: "Nov", day: "28" }}
          time={times.meetup}
          organizer="By Aoi"
          state={{ kind: "canceled", label: "Cancelled" }}
        />
      </RowItem>
      <RowItem>
        <EventLine
          href="/events/6"
          title="Sketch night"
          date={{ month: "Nov", day: "9" }}
          time={times.past}
          places={[{ kind: "physical", label: "In person" }]}
          organizer="By Kai"
          going={{ shown: "7", label: "7 went" }}
          state={{ kind: "ended", label: "Ended" }}
        />
      </RowItem>
    </RowList>
  );
}

/** The reader's answer, held here as the screen would hold it, with the save state the screen gives. */
function Answering({
  initial = null,
  save,
  ...rest
}: {
  initial?: Answer | null;
  save?: ReactNode | null;
  note?: string;
  isDisabled?: boolean;
  disabled?: readonly Answer[];
}) {
  const [value, setValue] = useState<Answer | null>(initial);
  return <AnswerChoice label="Your answer" names={going} value={value} onChange={setValue} save={save} {...rest} />;
}

type PollDate = {
  id: string;
  time: ReactNode;
  counts: Record<Answer, string>;
  answer: Answer | null;
  confirmed?: boolean;
};

const dates: readonly PollDate[] = [
  {
    id: "a",
    time: (
      <EventTime
        dateTime="2026-12-05T09:00:00Z"
        shown="Sat, Dec 5, 1:00 PM"
        own={{ shown: "Sat 6:00 PM", zone: "Tokyo" }}
      />
    ),
    counts: { yes: "5", maybe: "2", no: "1" },
    answer: "yes",
  },
  {
    id: "b",
    time: <EventTime dateTime="2026-12-06T09:00:00Z" shown="Sun, Dec 6, 1:00 PM" />,
    counts: { yes: "3", maybe: "4", no: "1" },
    answer: "maybe",
  },
  {
    id: "c",
    time: <EventTime dateTime="2026-12-12T09:00:00Z" shown="Sat, Dec 12, 1:00 PM" />,
    counts: { yes: "1", maybe: "0", no: "6" },
    answer: "no",
  },
  {
    id: "d",
    time: <EventTime dateTime="2026-12-13T02:00:00Z" shown="Sat, Dec 12, 6:00 PM" />,
    counts: { yes: "0", maybe: "0", no: "0" },
    answer: null,
  },
];

/** What an organizer may do with a date: confirm it, or withdraw it, last and apart. */
function DateActions({ name }: { name: string }) {
  return (
    <MenuSurface
      trigger={<IconButton icon="more" label={`Options for ${name}`} square />}
      label={`Options for ${name}`}
    >
      <MenuGroup>
        <MenuItem id="confirm" icon="dateFixed">
          Confirm this date…
        </MenuItem>
      </MenuGroup>
      <MenuGroup>
        <MenuItem id="withdraw" icon="delete">
          Withdraw this date…
        </MenuItem>
      </MenuGroup>
    </MenuSurface>
  );
}

const names = ["Sat, Dec 5", "Sun, Dec 6", "Sat, Dec 12, 1:00 PM", "Sat, Dec 12, 6:00 PM"];

/** The date poll, its answers held here as the screen would hold them. */
function Poll({
  organizer = false,
  closed = false,
  save,
}: {
  organizer?: boolean;
  closed?: boolean;
  save?: ReactNode | null;
}) {
  const [answers, setAnswers] = useState<Record<string, Answer | null>>(() =>
    Object.fromEntries(dates.map((d) => [d.id, d.answer])),
  );
  return (
    <CandidateTable
      label="Date poll"
      time="Date"
      names={poll}
      yours="Your answer"
      actions={organizer && !closed ? "Actions" : undefined}
      save={save}
    >
      {dates.map((d, i) => (
        <CandidateRow
          key={d.id}
          time={d.time}
          counts={d.counts}
          answer={answers[d.id] ?? null}
          onAnswer={(a) => {
            setAnswers((all) => ({ ...all, [d.id]: a }));
          }}
          isDisabled={closed}
          confirmed={closed && i === 0 ? "Confirmed" : undefined}
          actions={organizer && !closed ? <DateActions name={names[i] ?? ""} /> : undefined}
        />
      ))}
    </CandidateTable>
  );
}

/** The places of an event, each kind, as a participant the organizers approved sees them. */
function Places({ room }: { room: "waiting" | "open" | "unavailable" }) {
  return (
    <PlaceList label="Where">
      {room === "waiting" && <PlaceLine kind="voice" label="Voice chat room" note="Opens at the start" />}
      {room === "open" && (
        <PlaceLine
          kind="voice"
          label="Voice chat room"
          note="Open now"
          action={
            <LinkButton href="/voice/1" size="timeline">
              Join the room
            </LinkButton>
          }
        />
      )}
      {room === "unavailable" && (
        <PlaceLine
          kind="voice"
          label="Voice chat room"
          unavailable
          note="Could not open: the organizer was hosting another room"
        />
      )}
      <PlaceLine
        kind="physical"
        label="In person"
        name="Community hall, room 3"
        address={"1-2-3 Jingumae\nShibuya, Tokyo 150-0001"}
        link={{ href: "https://maps.example.com/?q=community+hall", text: "Open the map", host: "maps.example.com" }}
      />
      <PlaceLine
        kind="url"
        label="Online"
        name="The stream"
        link={{ href: "https://video.example.org/live/bookclub", text: "", host: "video.example.org" }}
      />
    </PlaceList>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Events", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * An event's page, composed of the parts: its title, its time, its places, the reader's answer, and, below, the poll
 * of another event's date, as a participant sees them.
 */
export const EventPage: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Stack gap="entry-line">
          <ScreenTitle>Book club: The Left Hand of Darkness</ScreenTitle>
          <Box text="body">{times.run}</Box>
          <Row wrap align="center" gapX="entry-caption" text="caption" tone="muted">
            <span>By Mika</span>
            <HeadcountMark count={{ shown: "12", label: "12 going" }} />
            <HeadcountMark count={{ shown: "3", label: "3 maybe" }} glyph="answerMaybe" />
          </Row>
        </Stack>
        <Places room="waiting" />
        <Answering save={null} />
        <Stack gap="entry-line">
          <GroupHeading>When can you go?</GroupHeading>
          <Poll save={null} />
        </Stack>
      </Stack>
    </Column>
  ),
};

/**
 * Events as lines of a list: a date fixed, its own zone beside the reader's, every kind of place, who organizes it and
 * how many are going; one place; a date polled, the glyph of events in the date's box; a title and a name that wrap;
 * cancelled and ended, each marked by a glyph and its words.
 */
export const EventLines: Story = {
  render: () => (
    <Column>
      <Lines />
    </Column>
  ),
};

/** The column beside the main one: the upcoming events, each a section's line with its time under its title. */
export const InTheColumnBeside: Story = {
  render: () => (
    <Aside>
      <FrameSection id="events" icon="events" href="/events" heading="Upcoming events">
        <SectionList>
          <SectionLine href="/events/1" title="Book club: The Left Hand of Darkness" caption={times.run} />
          <SectionLine href="/events/2" title="Release party for v0.2" caption={times.release} />
          <SectionLine href="/events/4" title="Winter meetup" caption="Date to be decided" />
        </SectionList>
      </FrameSection>
    </Aside>
  ),
};

/**
 * An event's time: in the reader's zone alone, where the event's is the same; over the event's own, with its zone's
 * place name, on a line of its own; in a body's size and in a caption's.
 */
export const Times: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Box text="body">{times.release}</Box>
        <Box text="body">{times.run}</Box>
        <Box text="caption" tone="muted">
          {times.run}
        </Box>
      </Stack>
    </Column>
  ),
};

/** The reader's answer before they answer: none chosen. */
export const AnswerUnanswered: Story = {
  render: () => (
    <Column>
      <Answering save={null} />
    </Column>
  ),
};

/** Answered: going, knocked out; then maybe, saving, the save state in the note's place; saved; and not saved. */
export const AnswerSaving: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Answering initial="yes" save={null} />
        <Answering initial="maybe" save={<SaveState state="saving" />} />
        <Answering initial="maybe" save={<SaveState state="saved" />} />
        <Answering initial="no" save={<SaveState state="failed" onRetry={none} />} />
      </Stack>
    </Column>
  ),
};

/** At capacity: going cannot be chosen, maybe and not going can, and the note says why. */
export const AnswerAtCapacity: Story = {
  render: () => (
    <Column>
      <Answering disabled={["yes"]} note="The event is full: no one more can go." save={null} />
    </Column>
  ),
};

/** No answer may be given: before a date is fixed, and, answered, after the start. */
export const AnswerDisabled: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Answering isDisabled note="You can answer once a date is fixed." />
        <Answering initial="yes" isDisabled note="The event has started: answers can no longer change." />
      </Stack>
    </Column>
  ),
};

/**
 * The date poll for a participant: a date a row, its counts, and the reader's answer, which saves as it is pressed; a
 * date in a zone apart from the reader's; one no one answered yet. A phone stacks each date: its time, its counts with
 * their glyphs, then the answers with their words.
 */
export const DatePoll: Story = {
  render: () => (
    <Column>
      <Poll save={null} />
    </Column>
  ),
};

/** The date poll for an organizer: each date's actions (confirm, withdraw) at the end of its row; the answer saving. */
export const DatePollForOrganizer: Story = {
  render: () => (
    <Column>
      <Poll organizer save={<SaveState state="saved" />} />
    </Column>
  ),
};

/** The date poll once a date is confirmed: the date marked, the answers held, no actions. */
export const DatePollConfirmed: Story = {
  render: () => (
    <Column>
      <Poll organizer closed />
    </Column>
  ),
};

/** An organizer's action menu on a date, open. */
export const DateActionsOpen: Story = {
  render: () => (
    <Column>
      <div style={{ minHeight: rem(320) }}>
        <MenuSurface
          trigger={<IconButton icon="more" label="Options for Sat, Dec 5" square />}
          label="Options for Sat, Dec 5"
          placement="bottom end"
          open
        >
          <MenuGroup>
            <MenuItem id="confirm" icon="dateFixed">
              Confirm this date…
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="withdraw" icon="delete">
              Withdraw this date…
            </MenuItem>
          </MenuGroup>
        </MenuSurface>
      </div>
    </Column>
  ),
};

/**
 * The places, before the start: the voice room opens at the start; in person, its name, its address on its lines and
 * its map, a link that leaves the site; online, its link, with the site it leads to.
 */
export const PlacesBeforeTheStart: Story = {
  render: () => (
    <Column>
      <Places room="waiting" />
    </Column>
  ),
};

/** The voice room open, with the way in; and one that could not open, its microphone crossed out, saying why. */
export const PlacesRoomOpenOrNot: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <Places room="open" />
        <Places room="unavailable" />
      </Stack>
    </Column>
  ),
};

/**
 * Places the reader may not see (shown to the approved participants): the kind alone, muted; the voice room
 * always shows. In person without a map.
 */
export const PlacesHidden: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <PlaceList label="Where">
          <PlaceLine kind="voice" label="Voice chat room" note="Opens at the start" />
          <PlaceLine kind="physical" label="In person" hidden />
          <PlaceLine kind="url" label="Online" hidden />
        </PlaceList>
        <PlaceList label="Where">
          <PlaceLine kind="physical" label="In person" name="The park by the station" address="North exit" />
        </PlaceList>
      </Stack>
    </Column>
  ),
};
