// SPDX-License-Identifier: AGPL-3.0-only

// The board of an event (ADR-0161 §5, §11, §17; #3354), in the app's shell: the event's page with its board for a
// reader who writes on it, empty, read-only (cancelled, or ended more than 7 days ago), for a reader who reads but does
// not write, and for one who may not read it; who writes, in the event's form, for its organizer and as said to a
// co-organizer; and the notification of a new post on the board. Each page is whole and edge to edge, so that it reads
// at the phone, tablet and PC widths of the toolbar's viewport.
//
// The board is the second of two tabs under the event's head, Details and Board, its address (the notification's)
// opening its tab; a reader who may not read it gets no tabs, the page as it is without a board. A writer writes from
// one bar at the board's top, their avatar and the prompt, which opens the app's composer, the event named over the
// text as a reply's post is. Where the bar would be, a reader who does not write now is told why.
//
// How it moves (the motion decisions: screens, states, frequency); with reduced motion nothing moves:
//
// - Details and Board are tabs of one screen, as the notifications' and a profile's: the panel changes at once, the
//   head and the row stay, kind `none`. The board's address is the event's with a search alone (`?tab=board`), which
//   navigationKind reads as `none`; a path of its own would read as a push. From the notification an event is deeper,
//   `push` (300 ms), and Back `pop` (250 ms); from its Web Push it is the first screen, `none`.
// - The bar opens the app's composer as a `modal` (ModalPresence: 300 ms, rising `transition.rise`), from the app's
//   state with no address; posting, saving an edit and cancelling close it the same way back. The bar: rest on the
//   raised surface, the surface's hover and press shades, the press's 1px shift (`translate-y-press`), the focus
//   ring; no busy, since the composer opens at once.
// - A post the reader posted, edited or deleted, and a page the feed loads near its end, come in, change or go at
//   once, as in a timeline (ADR-0013): the new post at the board's top, where the reader is when the composer closes,
//   the focus back on the bar; an edit and a deletion in their place, said by the post's actions' notice. Deleting
//   asks first in the post's confirmation (the dialog's motion); a failure is said with Try again.
// - What the board shows changes in place at once: the bar gives way to the read-only line when the event is
//   cancelled meanwhile, the focus on the bar going to the Board tab. A reader who loses the board on its tab is on
//   the page without tabs at once, the address replaced by the event's, the focus on the screen's heading.
// - The answer that gives or takes the board saves as it does (its SaveState), and once it is saved the tabs come or
//   go at once, the page as it is laid out (the answers move by the row), the focus staying on the answer pressed.
// - Who writes is a ChoiceList: hover, the focus ring, the checked mark, at once.
//
// The bar is the design system's PromptBar. A post of the board (the app's components' Post, whose actions have no
// boost and whose More has no bookmark) is drawn here as a stand-in, since the design system draws no post; the words
// are the catalog's in English.

import { color } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Avatar } from "./avatar";
import { ChoiceList } from "./choice";
import { ActionRow, Byline, Entry, Stamp } from "./entry";
import { AnswerChoice, EventTime, PlaceLine, PlaceList, type Answer } from "./event";
import { Feed, FeedItem, FeedNotice } from "./feed";
import { IconButton } from "./icon-button";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack } from "./layout";
import {
  AccountRow,
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
import { ShowMore } from "./pager";
import { PromptBar } from "./prompt-bar";
import { SaveState } from "./save-state";
import { HeadcountMark } from "./voice";

const minute = 60_000;
const hour = 60 * minute;
const none = () => undefined;

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

/** An account's line of a list, as AccountLine of the app's components draws it. */
function Person({ person }: { person: Person }) {
  return (
    <AccountRow
      avatar={<Avatar name={person.name} src={person.avatar} size="post" decorative />}
      name={person.name}
      handle={person.handle}
      href={`/${person.handle}`}
    />
  );
}

/** A list of people under its heading, its first page, and Show more under it while the server holds more. */
function People({ heading, people, more = false }: { heading: string; people: Person[]; more?: boolean }) {
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
      <ShowMore more={more} onPress={none} />
    </Stack>
  );
}

/** The state of the event the board follows: open, cancelled, or ended more than 7 days ago. */
type EventState = "open" | "cancelled" | "ended";

const going = { yes: "Going", maybe: "Maybe", no: "Not going" } as const;

/** The reader's answer, held here as the screen would hold it. */
function Answering({ initial, isDisabled }: { initial: Answer | null; isDisabled?: boolean }) {
  const [value, setValue] = useState<Answer | null>(initial);
  return (
    <AnswerChoice
      label="Your answer"
      names={going}
      value={value}
      onChange={setValue}
      save={null}
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
          <HeadcountMark count={{ shown: "24", label: "24 going" }} />
          <HeadcountMark count={{ shown: "2", label: "2 maybe" }} glyph="answerMaybe" />
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

/** Who goes: the first page of the 24 (the screen's page holds 20), and Show more under it. */
const goingPeople: Person[] = [
  sora,
  aoi,
  yuki,
  ...Array.from({ length: 17 }, (_, i) => ({ name: `Guest ${String(i + 1)}`, handle: `@guest${String(i + 1)}` })),
];

/** Who goes, and both who answered maybe. */
function Attendees() {
  return (
    <>
      <People heading="Going" people={goingPeople} more />
      <People
        heading="Maybe"
        people={[
          { name: "Haruto", handle: "@haruto", avatar: images.dusk },
          { name: "Lina", handle: "@lina" },
        ]}
      />
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

/** The bar at the board's top that opens the app's composer (PromptBar). */
function OpensComposer() {
  return (
    <PromptBar avatar={<Avatar name={yuki.name} src={images.snow} size="post" decorative />} onPress={none}>
      Write on the board
    </PromptBar>
  );
}

/** Who reads the board and whether the reader writes on it. */
type Access = "writer" | "reader" | "none";

/** Where the composer would be, for a reader who does not write now: why, in muted words. */
function NoComposer({ state, access }: { state: EventState; access: Access }) {
  const why =
    state === "cancelled"
      ? "The event was cancelled, so the board is read-only."
      : state === "ended"
        ? "7 days have passed since the event ended, so the board is read-only."
        : access === "reader"
          ? "Only the organizer and the co-organizers can write on this board."
          : undefined;
  if (why === undefined) return null;
  return (
    <Box as="p" text="caption" tone="muted">
      {why}
    </Box>
  );
}

function Board({ state, access, empty }: { state: EventState; access: Access; empty: boolean }) {
  const writes = access === "writer" && state === "open";
  const posts = empty ? [] : boardPosts;
  return (
    <Stack gap="entry-room">
      {writes ? <OpensComposer /> : <NoComposer state={state} access={access} />}
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
function Tabbed({
  details,
  board,
  initial = "board",
}: {
  details: ReactNode;
  board: ReactNode;
  initial?: "details" | "board";
}) {
  const [current, setCurrent] = useState<"details" | "board">(initial);
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

type PageProps = { state?: EventState; access?: Access; empty?: boolean };

/** The event's page: its head, then Details and Board as tabs; no tabs for a reader who may not read the board. */
function EventPage({ state = "open", access = "writer", empty = false }: PageProps) {
  const details = (
    <>
      <About />
      <Answering initial={access === "none" ? "no" : "yes"} isDisabled={state !== "open"} />
      <Organizers />
      <Attendees />
    </>
  );
  return (
    <Shell current="/events">
      <ScreenStack>
        <Head state={state} />
        {access === "none" ? (
          details
        ) : (
          <Tabbed details={details} board={<Board state={state} access={access} empty={empty} />} />
        )}
      </ScreenStack>
    </Shell>
  );
}

/**
 * The reader on Details, a writer going: Not going takes the board once it is saved (600 ms here), Going or Maybe
 * gives it back. The tabs go or come at once and the page is as it is laid out, the answers moving by the row; the
 * focus stays on the answer pressed.
 */
function AnswerChangesBoard() {
  const id = useId();
  const [answer, setAnswer] = useState<Answer>("yes");
  const [saving, setSaving] = useState<"saving" | "saved" | null>(null);
  const [reads, setReads] = useState(true);
  const focused = useRef(false);
  useLayoutEffect(() => {
    if (focused.current) document.getElementById(id)?.querySelector<HTMLInputElement>("input:checked")?.focus();
    focused.current = false;
  }, [id, reads]);
  const onChange = (a: Answer) => {
    setAnswer(a);
    setSaving("saving");
    window.setTimeout(() => {
      focused.current = document.getElementById(id)?.contains(document.activeElement) ?? false;
      setSaving("saved");
      setReads(a !== "no");
    }, 600);
  };
  const details = (
    <>
      <About />
      <Stack id={id} gap="entry-line">
        <GroupHeading>Your answer</GroupHeading>
        <AnswerChoice
          label="Your answer"
          names={going}
          value={answer}
          onChange={onChange}
          save={saving === null ? null : <SaveState state={saving} />}
        />
      </Stack>
      <Organizers />
      <Attendees />
    </>
  );
  return (
    <Shell current="/events">
      <ScreenStack>
        <Head state="open" />
        {reads ? (
          <Tabbed initial="details" details={details} board={<Board state="open" access="writer" empty={false} />} />
        ) : (
          details
        )}
      </ScreenStack>
    </Shell>
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
      <GroupHeading>Who can write on the board</GroupHeading>
      <ChoiceList
        label="Who can write on the board"
        multiple={false}
        items={writers.map((key) => ({ key, label: writerWords[key] }))}
        chosen={[chosen]}
        onChange={([i]) => {
          if (i !== undefined) setChosen(i);
        }}
      />
      <Box as="p" text="caption" tone="muted">
        You, the co-organizers, and the people who answered going or maybe can read the board.
      </Box>
    </Stack>
  );
}

/** The same, said to a co-organizer: the organizer's to change, as who sees the event. */
function WritersSaid() {
  return (
    <Stack gap="entry-line">
      <GroupHeading>Who can write on the board</GroupHeading>
      <Box as="p" text="body">
        The organizer and the co-organizers can write on the board. Only the organizer, Mika, changes who can write on
        it.
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

/** A reader going, who writes on the board (everyone who reads it writes): the bar that opens the composer, its posts. */
export const Writer: Story = { render: () => <EventPage /> };

/** The same board with nothing on it yet. */
export const Empty: Story = { render: () => <EventPage empty /> };

/** A reader of a board only the organizers write on: no bar, and who writes, in its place. */
export const ReaderNotWriter: Story = { render: () => <EventPage access="reader" /> };

/** A cancelled event: the board read-only, said where the bar would be; the answers closed. */
export const ReadOnlyCancelled: Story = { render: () => <EventPage state="cancelled" /> };

/** An event that ended more than 7 days ago: the board read-only, said where the bar would be. */
export const ReadOnlyEnded: Story = { render: () => <EventPage state="ended" /> };

/** A reader who sees the event and answered Not going: no tabs and nothing of the board, the page as it is. */
export const NoAccess: Story = { render: () => <EventPage access="none" /> };

/** Who writes on the board, in the form that changes the event, for its organizer: after who sees it. */
export const WritersInTheForm: Story = { render: () => <FormEnd organizer /> };

/** The same for a co-organizer: who writes, said, the organizer's to change. */
export const WritersSaidToCoOrganizer: Story = { render: () => <FormEnd organizer={false} /> };

/**
 * The reader on Details answers Not going, and once it is saved the tabs go at once, the answers moving up by the
 * row; Going or Maybe brings them back.
 */
export const AnswerChangesTheBoard: Story = { render: () => <AnswerChangesBoard /> };

/** The notifications of new posts on a board: the board's glyph, the writer, the event, the post's first words. */
export const Notification: Story = { render: () => <Notifications /> };
