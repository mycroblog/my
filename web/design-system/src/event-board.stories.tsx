// SPDX-License-Identifier: AGPL-3.0-only

// The board of an event (ADR-0161 §5, §11, §17; #3354), in the app's shell, in grays: the event's page with its board
// for a reader who writes on it, empty, read-only (cancelled, or ended more than 7 days ago), for a reader who reads but
// does not write, and for one who may not read it; who writes, in the event's form, for its organizer and as said to a
// co-organizer; and the notification of a new post on the board. Each page is whole and edge to edge, so that it reads
// at the phone, tablet and PC widths of the toolbar's viewport. Live options:
//
// - `event-board-place`, where the board stands on the event's page: `after`, a section after everything the page
//   holds (its time, places, the reader's answer, its organizers and who goes); `middle`, a section right after the
//   reader's answer, before its organizers and who goes; `tabs`, two tabs under the event's head, Details and Board,
//   the address of the board (the notification's) opening its tab.
// - `event-board-composer`, how a writer writes on it: `in-place`, a box at the board's top on the raised surface, as
//   the reply in place of a conversation, with the tools of a post but boost and quote (attach, poll, content warning,
//   language, emoji); `dialog`, a row at the board's top that opens the composer, the event named above its text, as
//   a reply's post is.
// - `event-board-closed`, what a reader who may not read the board sees: `none`, nothing of it; `hint`, the line under
//   the answers saying that going or maybe opens it.
//
// What the design system lacks yet (a post of the board, whose actions have no boost, the board's composer, an
// account's line) is drawn here as a stand-in, and the words are samples in English.

import { color } from "@my/design-tokens";
import { Options } from "@my/devtools/options";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLayoutEffect, useState, type ReactNode } from "react";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { ChoiceList } from "./choice";
import { CharCount, ComposerText } from "./composer";
import { PlainTextEditor } from "./editor";
import { ActionRow, Byline, Entry, Stamp } from "./entry";
import { AnswerChoice, EventTime, PlaceLine, PlaceList, type Answer } from "./event";
import { Feed, FeedItem, FeedNotice } from "./feed";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack } from "./layout";
import {
  BackButton,
  Excerpt,
  GroupHeading,
  NoticeLine,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  SentenceName,
  TabRow,
} from "./list";
import { ago, images } from "./samples";
import {
  Frame,
  FrameAction,
  FrameActions,
  FrameAside,
  FrameDock,
  FrameFooter,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  NavItem,
  NavList,
  NavMenu,
  SectionLine,
  SectionList,
} from "./shell";
import { HeadcountMark } from "./voice";

type Place = "after" | "middle" | "tabs";
type Writing = "in-place" | "dialog";
type Closed = "none" | "hint";

const minute = 60_000;
const hour = 60 * minute;
const none = () => undefined;

/** The page in grays while the board is settled: what is on it, its order and its grouping, before any color. */
function useGrays() {
  useLayoutEffect(() => {
    const root = document.documentElement;
    root.style.filter = "grayscale(1)";
    return () => {
      root.style.filter = "";
    };
  }, []);
}

// ---- The app's shell around the page --------------------------------------------------------------------------------

// A stand-in for the brand's wordmark, in the text's color of a scheme.
const wordmark = (fill: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 44 24" width="44" height="24"><text x="2" y="19" font-family="system-ui, sans-serif" font-size="21" font-style="italic" font-weight="800" fill="${fill}">my</text></svg>`)}`;

type NavPlace = { href: string; icon: IconName; label: string; count?: number };

const tabs: NavPlace[] = [
  { href: "/", icon: "home", label: "Home" },
  { href: "/local", icon: "local", label: "Local" },
  { href: "/public", icon: "federated", label: "Federated" },
  { href: "/notifications", icon: "notifications", label: "Notifications", count: 3 },
  { href: "/search", icon: "search", label: "Search" },
];
const rest: NavPlace[] = [
  { href: "/direct", icon: "directMessages", label: "Direct messages" },
  { href: "/@yuki", icon: "profile", label: "Profile" },
  { href: "/bookmarks", icon: "bookmark", label: "Bookmarks" },
  { href: "/settings", icon: "settings", label: "Settings" },
];
const server: NavPlace[] = [
  { href: "/announcements", icon: "announcements", label: "Announcements" },
  { href: "/voice", icon: "room", label: "Voice rooms" },
  { href: "/events", icon: "events", label: "Events" },
];

const places = (items: NavPlace[], current: string) => (
  <NavList>
    {items.map((p) => (
      <NavItem
        key={p.href}
        href={p.href}
        icon={p.icon}
        label={p.label}
        current={p.href === current}
        count={p.count}
        countLabel={p.count === undefined ? undefined : `${String(p.count)} unread`}
      />
    ))}
  </NavList>
);

const footer = (
  <FrameFooter
    label="This server"
    links={[
      { href: "/about", label: "About", icon: "about" },
      { href: "/terms", label: "Terms", icon: "file" },
      { href: "/privacy", label: "Privacy", icon: "file" },
    ]}
  />
);

/** The app's shell, at `current` (a place's address), around the main column's `children`. */
function Shell({ current, children }: { current: string; children: ReactNode }) {
  useGrays();
  return (
    <Frame>
      <FrameSide label="Main">
        <FrameMark href="/" label="my.example" light={wordmark(color.text.light)} dark={wordmark(color.text.dark)} />
        <FrameActions>
          <FrameAction icon="compose" label="New post" onPress={none} />
          <FrameAction icon="article" label="New article" secondary onPress={none} />
        </FrameActions>
        {places([...tabs, ...rest], current)}
        <NavList>
          <NavMenu label="Menu" footer={footer}>
            {places(server, current)}
          </NavMenu>
        </NavList>
      </FrameSide>
      <FrameMain>{children}</FrameMain>
      <FrameAside label="Server activity">
        <FrameSection id="around-events" icon="events" href="/events" heading="Upcoming events">
          <SectionList>
            <SectionLine href="/events/1" title="Book club: The Left Hand of Darkness" caption="Sat, Nov 28, 2:00 PM" />
            <SectionLine href="/events/2" title="Release party for v0.2" caption="Mon, Nov 30, 7:00 PM" />
          </SectionList>
        </FrameSection>
        {footer}
      </FrameAside>
      <FrameDock
        label="Main"
        tabs={
          <NavList>
            {tabs.map((p) => (
              <NavItem
                key={p.href}
                href={p.href}
                icon={p.icon}
                label={p.label}
                current={p.href === current}
                count={p.count}
                countLabel={p.count === undefined ? undefined : `${String(p.count)} unread`}
              />
            ))}
            <NavMenu label="Menu" footer={footer}>
              {places(rest, current)}
              {places(server, current)}
            </NavMenu>
          </NavList>
        }
      />
    </Frame>
  );
}

// ---- The event's page -----------------------------------------------------------------------------------------------

type Person = { name: string; handle: string; avatar?: string | undefined };

const mika: Person = { name: "Mika", handle: "@mika", avatar: images.dusk };
const ren: Person = { name: "Ren", handle: "@ren", avatar: images.snow };
const sora: Person = { name: "Sora", handle: "@sora", avatar: images.logo };
const aoi: Person = { name: "Aoi Tanaka", handle: "@aoi" };
const yuki: Person = { name: "Yuki", handle: "@yuki" };

/** A stand-in for an account's line of a list (AccountLine of the app's components). */
function Person({ person }: { person: Person }) {
  return (
    <Row align="center" gap="entry-avatar">
      <Avatar name={person.name} src={person.avatar} size="row" decorative />
      <Stack shrink gap="hair">
        <Box as="bdi" lines={1} weight="label">
          {person.name}
        </Box>
        <Box as="bdi" lines={1} text="caption" tone="muted">
          {person.handle}
        </Box>
      </Stack>
    </Row>
  );
}

function People({ heading, people }: { heading: string; people: Person[] }) {
  return (
    <Stack gap="entry-line">
      <GroupHeading>{heading}</GroupHeading>
      <RowList label={heading}>
        {people.map((p) => (
          <RowItem key={p.handle}>
            <Person person={p} />
          </RowItem>
        ))}
      </RowList>
    </Stack>
  );
}

/** The state of the event the board follows: open, cancelled, or ended more than 7 days ago. */
type EventState = "open" | "cancelled" | "ended";

const going = { yes: "Going", maybe: "Maybe", no: "Not going" } as const;

/** The reader's answer, held here as the screen would hold it. */
function Answering({
  initial,
  note,
  isDisabled,
}: {
  initial: Answer | null;
  note?: string | undefined;
  isDisabled?: boolean;
}) {
  const [value, setValue] = useState<Answer | null>(initial);
  return (
    <AnswerChoice
      label="Your answer"
      names={going}
      value={value}
      onChange={setValue}
      save={null}
      note={note}
      isDisabled={isDisabled}
    />
  );
}

/** The event's head: back, its title and More, what became of it, its time, and who organizes it and how many go. */
function Head({ state }: { state: EventState }) {
  return (
    <>
      <ScreenTitle
        back={<BackButton onPress={none}>Events</BackButton>}
        actions={<IconButton icon="more" label="More for Picnic by the river" square />}
      >
        Picnic by the river
      </ScreenTitle>
      {state === "cancelled" && (
        <Row as="p" align="center" gap="entry-caption">
          <Icon name="eventCancelled" place="caption" />
          <span>Cancelled by Mika: “Rain all day, sorry.”</span>
        </Row>
      )}
      {state === "ended" && (
        <Row as="p" align="center" gap="entry-caption" tone="muted">
          <Icon name="eventEnded" place="caption" />
          <span>Ended</span>
        </Row>
      )}
      <Stack gap="entry-line">
        <Box text="body">
          <EventTime
            dateTime="2026-11-28T03:00:00Z"
            shown={state === "ended" ? "Sat, Nov 7, 12:00 – 4:00 PM" : "Sat, Nov 28, 12:00 – 4:00 PM"}
          />
        </Box>
        <Row wrap align="center" gapX="entry-caption" text="caption" tone="muted">
          <span>By Mika</span>
          <HeadcountMark count={{ shown: "8", label: "8 going" }} />
          <HeadcountMark count={{ shown: "3", label: "3 maybe" }} glyph="answerMaybe" />
        </Row>
      </Stack>
    </>
  );
}

/** What the event is: its description and its places. */
function About() {
  return (
    <>
      <Box as="p" text="body" dir="auto">
        Bring something to share and a blanket. We meet by the big willow, and move under the bridge if it rains.
      </Box>
      <Stack gap="entry-line">
        <GroupHeading>Where</GroupHeading>
        <PlaceList label="Where">
          <PlaceLine
            kind="physical"
            label="In person"
            name="Riverside park, by the big willow"
            address={"2-1 Kawabata\nSetagaya, Tokyo"}
          />
        </PlaceList>
      </Stack>
    </>
  );
}

function Organizers() {
  return <People heading="Organizers" people={[mika, ren]} />;
}

function Attendees() {
  return (
    <>
      <People heading="Going" people={[sora, aoi, yuki]} />
      <People heading="Maybe" people={[{ name: "Haruto", handle: "@haruto", avatar: images.dusk }]} />
    </>
  );
}

// ---- The board ------------------------------------------------------------------------------------------------------

type BoardPost = {
  id: string;
  by: Person;
  at: number;
  text: string;
  edited?: boolean;
  counts?: { replies?: number; favourites?: number };
};

const boardPosts: BoardPost[] = [
  {
    id: "b1",
    by: mika,
    at: 40 * minute,
    text: "The forecast says light rain after 3. If it starts, we move under the bridge, 100 m downstream.",
    edited: true,
    counts: { replies: 2, favourites: 5 },
  },
  {
    id: "b2",
    by: sora,
    at: 3 * hour,
    text: "I can bring a big thermos of tea and paper cups for everyone.",
    counts: { favourites: 3 },
  },
  {
    id: "b3",
    by: ren,
    at: 26 * hour,
    text: "From the station, take the north exit and follow the river path. The willow is past the second bench.",
    counts: { replies: 1, favourites: 7 },
  },
];

/** A stand-in for a post of the board: the post as every post, its actions reply, favourite, react and more. */
function Post({ post }: { post: BoardPost }) {
  const name = `${post.id}-name`;
  return (
    <Entry labelledBy={name}>
      <Byline
        avatar={<Avatar name={post.by.name} src={post.by.avatar} size="post" decorative />}
        name={post.by.name}
        nameId={name}
        handle={post.by.handle}
        end={
          <Stamp
            glyph={{ icon: "board", label: "On the event's board" }}
            date={ago(post.at)}
            edited={post.edited === true ? { text: "(edited)", label: "Edited", onPress: none } : undefined}
          />
        }
      />
      <Box as="p" dir="auto">
        {post.text}
      </Box>
      <ActionRow end={<IconButton icon="more" label="More" />}>
        <IconButton icon="reply" label="Reply" count={post.counts?.replies} />
        <IconButton icon="favourite" label="Favorite" count={post.counts?.favourites} />
        <IconButton icon="react" label="React" />
      </ActionRow>
    </Entry>
  );
}

/** The box a writer writes in at the board's top, as the reply in place of a conversation. */
function InPlace() {
  return (
    <Stack
      as="form"
      gap="entry"
      surface="raised"
      radius="entry"
      pad="embed"
      aria-label="Write on the board"
      onSubmit={(e) => {
        e.preventDefault();
      }}
    >
      <ComposerText roomy={false}>
        <PlainTextEditor
          namespace="board"
          look="composer"
          value=""
          onChange={none}
          label="Your post on the board"
          placeholder="Write to the people of this event"
        />
      </ComposerText>
      <Row align="center" gap="entry-caption">
        <Row grow shrink align="center" bleedStart="icon-button-inline">
          <IconButton icon="attach" label="Add images, a video or a sound" />
          <IconButton icon="poll" label="Add a poll" />
          <IconButton icon="hidden" label="Add a content warning" />
          <IconButton icon="translate" label="Language: English" />
          <IconButton icon="emojiSmileys" label="Add an emoji" />
        </Row>
        <CharCount left={500} near={20} label="500" />
        <Button kind="primary" size="timeline">
          Post
        </Button>
      </Row>
    </Stack>
  );
}

/** The row at the board's top that opens the composer, the event named over its text. */
function OpensComposer() {
  return (
    <Row align="center" gap="entry-avatar" surface="raised" radius="entry" pad="embed">
      <Avatar name={yuki.name} size="row" decorative />
      <Box grow tone="muted">
        Write to the people of this event
      </Box>
      <Button size="timeline">Write</Button>
    </Row>
  );
}

/** Who reads the board and whether the reader writes on it. */
type Access = "writer" | "reader" | "none";

/** Where the composer would be, for a reader who does not write now: why, in muted words. */
function NoComposer({ state, access }: { state: EventState; access: Access }) {
  const why =
    state === "cancelled"
      ? "The event was cancelled: the board is read-only."
      : state === "ended"
        ? "The event ended more than 7 days ago: the board is read-only."
        : access === "reader"
          ? "Only the organizers write on this board."
          : undefined;
  if (why === undefined) return null;
  return (
    <Box as="p" text="caption" tone="muted">
      {why}
    </Box>
  );
}

function Board({
  writing,
  state,
  access,
  empty,
}: {
  writing: Writing;
  state: EventState;
  access: Access;
  empty: boolean;
}) {
  const writes = access === "writer" && state === "open";
  const posts = empty ? [] : boardPosts;
  return (
    <Stack gap="entry-room">
      {writes ? writing === "in-place" ? <InPlace /> : <OpensComposer /> : <NoComposer state={state} access={access} />}
      <Feed
        label="The event's board"
        count={posts.length}
        more={false}
        busy={false}
        end={posts.length === 0 ? <FeedNotice>Nothing on the board yet.</FeedNotice> : undefined}
      >
        {posts.map((p, i) => (
          <FeedItem key={p.id} id={p.id} position={i + 1}>
            <Post post={p} />
          </FeedItem>
        ))}
      </Feed>
    </Stack>
  );
}

/** The tabs of `tabs`, the board's current at first, as its address (the notification's) opens it. */
function Tabbed({ details, board }: { details: ReactNode; board: ReactNode }) {
  const [current, setCurrent] = useState<"details" | "board">("board");
  return (
    <TabRow
      label="Picnic by the river"
      tabs={[
        { id: "details", name: "Details" },
        { id: "board", name: "Board" },
      ]}
      current={current}
      onChange={setCurrent}
    >
      {current === "board" ? board : <ScreenStack>{details}</ScreenStack>}
    </TabRow>
  );
}

type PageProps = {
  place: Place;
  writing: Writing;
  closed: Closed;
  state?: EventState;
  access?: Access;
  empty?: boolean;
};

/** The event's page, the board where `place` puts it. */
function EventPage({ place, writing, closed, state = "open", access = "writer", empty = false }: PageProps) {
  const reads = access !== "none";
  const answer = (
    <Answering
      initial={access === "none" ? "no" : "yes"}
      isDisabled={state !== "open"}
      note={!reads && closed === "hint" ? "Answer Going or Maybe to read the event's board." : undefined}
    />
  );
  const board = reads && <Board writing={writing} state={state} access={access} empty={empty} />;
  const section = board !== false && (
    <Stack gap="entry-line">
      <GroupHeading>Board</GroupHeading>
      {board}
    </Stack>
  );
  const details = (
    <>
      <About />
      {answer}
      <Organizers />
      <Attendees />
    </>
  );
  return (
    <Shell current="/events">
      <ScreenStack>
        <Head state={state} />
        {place === "tabs" ? (
          board === false ? (
            details
          ) : (
            <Tabbed details={details} board={board} />
          )
        ) : (
          <>
            <About />
            {answer}
            {place === "middle" && section}
            <Organizers />
            <Attendees />
            {place === "after" && section}
          </>
        )}
      </ScreenStack>
    </Shell>
  );
}

/** The page in each option of where the board stands and how a writer writes on it. */
const page = (props: Omit<PageProps, "place" | "writing" | "closed">) => (
  <Options
    id="event-board-place"
    variants={{
      after: <Writings place="after" {...props} />,
      middle: <Writings place="middle" {...props} />,
      tabs: <Writings place="tabs" {...props} />,
    }}
  />
);

function Writings(props: Omit<PageProps, "writing" | "closed">) {
  return (
    <Options
      id="event-board-composer"
      variants={{
        "in-place": <EventPage writing="in-place" closed="none" {...props} />,
        dialog: <EventPage writing="dialog" closed="none" {...props} />,
      }}
    />
  );
}

// ---- Who writes, in the event's form --------------------------------------------------------------------------------

const writers = ["organizer", "organizers", "readers"] as const;
const writerWords: Record<(typeof writers)[number], string> = {
  organizer: "Only you",
  organizers: "You and the co-organizers",
  readers: "Everyone who reads the board",
};

/** The choice of who writes on the board, as the form holds its other choices, after who sees the event. */
function WritersChoice() {
  const [chosen, setChosen] = useState(1);
  return (
    <Stack gap="entry-line">
      <GroupHeading>Who writes on the board</GroupHeading>
      <ChoiceList
        label="Who writes on the board"
        multiple={false}
        items={writers.map((key) => ({ key, label: writerWords[key] }))}
        chosen={[chosen]}
        onChange={([i]) => {
          if (i !== undefined) setChosen(i);
        }}
      />
      <Box as="p" text="caption" tone="muted">
        The organizers and those going or maybe read the board.
      </Box>
    </Stack>
  );
}

/** The same, said to a co-organizer: the organizer's to change, as who sees the event. */
function WritersSaid() {
  return (
    <Stack gap="entry-line">
      <GroupHeading>Who writes on the board</GroupHeading>
      <Box as="p" text="body">
        The organizer and the co-organizers. Mika, the organizer, changes it.
      </Box>
    </Stack>
  );
}

/** The end of the event's form: who sees it, then who writes on the board, then its capacity's place. */
function FormEnd({ organizer }: { organizer: boolean }) {
  const [visibility, setVisibility] = useState(0);
  const seen = ["Everyone on this server", "Your followers", "People you follow", "Mutual followers", "Invited only"];
  return (
    <Shell current="/events">
      <ScreenStack>
        <ScreenTitle back={<BackButton onPress={none}>Picnic by the river</BackButton>}>Change the event</ScreenTitle>
        <Stack gap="entry-room">
          {organizer ? (
            <Stack gap="entry-line">
              <GroupHeading>Who sees the event</GroupHeading>
              <ChoiceList
                label="Who sees the event"
                multiple={false}
                items={seen.map((label, key) => ({ key: String(key), label }))}
                chosen={[visibility]}
                onChange={([i]) => {
                  if (i !== undefined) setVisibility(i);
                }}
              />
            </Stack>
          ) : (
            <Stack gap="entry-line">
              <GroupHeading>Who sees the event</GroupHeading>
              <Box as="p" text="body">
                Everyone on this server. Mika, the organizer, changes it.
              </Box>
            </Stack>
          )}
          {organizer ? <WritersChoice /> : <WritersSaid />}
          <Box as="p" weight="label">
            Capacity
          </Box>
        </Stack>
      </ScreenStack>
    </Shell>
  );
}

// ---- The notification -----------------------------------------------------------------------------------------------

function Notice({
  icon,
  person,
  unread,
  at,
  children,
  excerpt,
}: {
  icon: IconName;
  person: Person;
  unread: boolean;
  at: number;
  children: ReactNode;
  excerpt?: string | undefined;
}) {
  const id = `${icon}-${person.handle}`;
  return (
    <Entry labelledBy={id}>
      <NoticeLine
        icon={icon}
        avatar={<Avatar name={person.name} src={person.avatar} size="inline" decorative />}
        date={ago(at)}
        id={id}
        unread={unread ? "Unread" : undefined}
        href="/events/1"
        action={<IconButton icon="close" label="Dismiss" />}
        excerpt={excerpt !== undefined && <Excerpt>{excerpt}</Excerpt>}
      >
        {children}
      </NoticeLine>
    </Entry>
  );
}

/** The notifications with a new post on a board, its first words under the sentence, among the event's others. */
function Notifications() {
  return (
    <Shell current="/notifications">
      <ScreenStack>
        <ScreenTitle>Notifications</ScreenTitle>
        <Feed label="Notifications" count={4} more={false} busy={false}>
          <FeedItem id="n1" position={1}>
            <Notice icon="board" person={mika} unread at={12 * minute} excerpt={boardPosts[0]?.text}>
              <SentenceName>Mika</SentenceName> wrote on the board of Picnic by the river
            </Notice>
          </FeedItem>
          <FeedItem id="n2" position={2}>
            <Notice icon="board" person={sora} unread at={3 * hour} excerpt={boardPosts[1]?.text}>
              <SentenceName>Sora</SentenceName> wrote on the board of Picnic by the river
            </Notice>
          </FeedItem>
          <FeedItem id="n3" position={3}>
            <Notice icon="eventReminder" person={mika} unread={false} at={20 * hour}>
              Picnic by the river starts in 1 day
            </Notice>
          </FeedItem>
          <FeedItem id="n4" position={4}>
            <Notice icon="events" person={mika} unread={false} at={5 * 24 * hour}>
              <SentenceName>Mika</SentenceName> invited you to Picnic by the river
            </Notice>
          </FeedItem>
        </Feed>
      </ScreenStack>
    </Shell>
  );
}

// ---- The stories ----------------------------------------------------------------------------------------------------

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Event board", tags: ["themes"], parameters: { layout: "fullscreen" } } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A reader going, who writes on the board (everyone who reads it writes): the composer at its top, its posts. */
export const Writer: Story = { render: () => page({}) };

/** The same board with nothing on it yet. */
export const Empty: Story = { render: () => page({ empty: true }) };

/** A reader of a board only the organizers write on: no composer, and who writes, in its place. */
export const ReaderNotWriter: Story = { render: () => page({ access: "reader" }) };

/** A cancelled event: the board read-only, said where the composer would be; the answers closed. */
export const ReadOnlyCancelled: Story = { render: () => page({ state: "cancelled" }) };

/** An event that ended more than 7 days ago: the board read-only, said where the composer would be. */
export const ReadOnlyEnded: Story = { render: () => page({ state: "ended" }) };

/**
 * A reader who sees the event and answered Not going: no board, in each option of what they are told; and in
 * each option of where the board would stand, which leaves no tabs.
 */
export const NoAccess: Story = {
  render: () => (
    <Options
      id="event-board-closed"
      variants={{
        none: <EventPage place="after" writing="in-place" closed="none" access="none" />,
        hint: <EventPage place="after" writing="in-place" closed="hint" access="none" />,
      }}
    />
  ),
};

/** Who writes on the board, in the form that changes the event, for its organizer: after who sees it. */
export const WritersInTheForm: Story = { render: () => <FormEnd organizer /> };

/** The same for a co-organizer: who writes, said, the organizer's to change. */
export const WritersSaidToCoOrganizer: Story = { render: () => <FormEnd organizer={false} /> };

/** The notifications of new posts on a board: the board's glyph, the writer, the event, the post's first words. */
export const Notification: Story = { render: () => <Notifications /> };
