// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import {
  CallBar,
  Frame,
  FrameAction,
  FrameAside,
  FrameBanner,
  FrameBanners,
  FrameDock,
  FrameFold,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  Meter,
  NavMenu,
  SectionLine,
  SectionList,
  SectionNote,
  NavItem,
  NavList,
  Toast,
} from "./shell";

const places = (
  <NavList>
    <NavItem href="/" icon="home" label="Home" current />
    <NavItem href="/notifications" icon="notifications" label="Notifications" count={3} countLabel="3 unread" />
  </NavList>
);

const call = (
  <CallBar
    href="/voice/1"
    room="Coffee"
    detail="Hosted by Mika · 12 in the room"
    muted
    muteLabel="Mute"
    onMute={() => undefined}
    leaveLabel="Leave"
    onLeave={() => undefined}
  />
);

const screen = renderToStaticMarkup(
  <Frame>
    <FrameSide label="Main">
      <FrameMark href="/" label="mycroblog" light="/light.svg" dark="/dark.svg" />
      <FrameAction icon="compose" label="New post" onPress={() => undefined} aria-keyshortcuts="N" />
      {places}
      {call}
    </FrameSide>
    <FrameMain>
      <FrameBanners label="Server and connection status">
        <FrameBanner icon="offline" links={[{ href: "https://status.example", label: "Status", newTab: "(new tab)" }]}>
          You are offline. What you see may be out of date until you reconnect.
        </FrameBanner>
      </FrameBanners>
      <FrameHeading>Home</FrameHeading>
      <FrameFold icon="announcements" href="/announcements">
        2 unread announcements
      </FrameFold>
    </FrameMain>
    <FrameAside label="Server activity">
      <FrameSection
        id="server"
        icon="about"
        href="/about"
        heading="About this server"
        all={{ text: "All", name: "All about this server" }}
      >
        <Meter value={1.2} label="Costs covered" caption="of the month" />
      </FrameSection>
      <FrameSection id="notifications" icon="notifications" href="/notifications" heading="Notifications">
        <SectionList>
          <SectionLine href="/@ren" icon="follow" title="Ren followed you" caption="1h ago" unread="Unread" />
        </SectionList>
        <SectionNote>No notifications yet.</SectionNote>
      </FrameSection>
    </FrameAside>
    <FrameDock
      label="Main"
      tabs={
        <NavList>
          <NavItem href="/" icon="home" label="Home" current />
          <NavMenu label="Menu" closeLabel="Close" marked markLabel="Something waits">
            <NavList>
              <NavItem href="/settings" icon="settings" label="Settings" />
            </NavList>
          </NavMenu>
        </NavList>
      }
      call={call}
      action={<FrameAction icon="compose" label="New post" onPress={() => undefined} />}
      toasts={
        <Toast action="Update" onAction={() => undefined} closeLabel="Close" onClose={() => undefined}>
          A newer version of the app is ready.
        </Toast>
      }
    />
  </Frame>,
);

test("the regions are landmarks with their names, and the screen's heading is the one h1", () => {
  expect(screen.match(/<nav [^>]*aria-label="Main"/g)).toHaveLength(2);
  expect(screen).toContain("<main ");
  expect(screen).toMatch(/<aside [^>]*aria-label="Server activity"/);
  expect(screen.match(/<h1 /g)).toHaveLength(1);
  expect(screen).toMatch(/<h1 [^>]*tabindex="-1"[^>]*>Home<\/h1>/);
});

test("the place where the reader is is current, and each place is named by its label, which the rail keeps for assistive technology", () => {
  expect(screen.match(/aria-current="page"/g)).toHaveLength(2);
  expect(screen).toMatch(
    /<a [^>]*aria-current="page"[^>]*>.*?<span class="sr-only desktop:not-sr-only[^"]*">Home<\/span>/,
  );
  expect(screen).not.toMatch(/<a [^>]*href="\/"[^>]*aria-label=/);
});

test("a place with a count is named by its label and the count's label, its badges hidden", () => {
  expect(screen).toMatch(/>Notifications<\/span> <span class="sr-only">3 unread<\/span>/);
  expect(screen.match(/<span class="[^"]*bg-text[^"]*" aria-hidden="true">3<\/span>/g)).toHaveLength(2);
});

test("the menu's place is a button that opens a dialog, marked while something inside waits", () => {
  expect(screen).toMatch(/<button [^>]*aria-haspopup="dialog"[^>]*aria-expanded="false"/);
  expect(screen).toMatch(/>Menu<\/span> <span class="sr-only">Something waits<\/span><\/button>/);
});

test("the call's bar in the dock marks the frame, and its microphone is a toggle named by its label", () => {
  expect(screen.match(/data-frame-call=""/g)).toHaveLength(1);
  expect(screen).toMatch(/<button [^>]*aria-pressed="true"[^>]*>.*?<span class="sr-only[^"]*">Mute<\/span><\/button>/);
});

test("a banner's link to another site opens a new tab and says so", () => {
  expect(screen).toMatch(/<a [^>]*target="_blank"[^>]*>Status<span class="sr-only"> \(new tab\)<\/span><\/a>/);
});

test("the meter is full at its goal and beyond, and reads the share", () => {
  expect(screen).toMatch(/role="meter"[^>]*aria-valuenow="120"[^>]*aria-valuetext="120%"/);
  expect(screen).toContain("inline-size:100%");
});

test("a toast is a status with its action and its close", () => {
  expect(screen).toMatch(/<div [^>]*role="status"><span[^>]*>A newer version of the app is ready\.<\/span><button/);
  expect(screen).toContain('aria-label="Close"');
});

test("the primary action is named by its label and carries its keys", () => {
  expect(screen).toMatch(
    /<button [^>]*aria-label="New post"[^>]*aria-keyshortcuts="N"|<button [^>]*aria-keyshortcuts="N"[^>]*aria-label="New post"/,
  );
});

test("a section is named by its heading alone, which links to its place, and its link to all of it has its own name", () => {
  expect(screen).toMatch(/<section [^>]*aria-labelledby="server"/);
  expect(screen).toMatch(/<h2 id="server"[^>]*><a [^>]*href="\/about"[^>]*>.*?About this server<\/a><\/h2>/);
  expect(screen).toMatch(/<\/h2><a [^>]*aria-label="All about this server"[^>]*>All<\/a>/);
});

test("a section folded above a feed is one link to its place, hidden where the column beside shows it", () => {
  expect(screen).toMatch(/<a class="[^"]*wide:hidden[^"]*" [^>]*href="\/announcements"[^>]*>.*?2 unread announcements/);
});

test("a section's line with a glyph links its title, marked while unread, and a note says what it shows instead", () => {
  expect(screen).toMatch(
    /<li [^>]*><span [^>]*><svg [^>]*aria-hidden="true"[^>]*>.*?<a [^>]*href="\/@ren"[^>]*>Ren followed you<\/a>/,
  );
  expect(screen).toMatch(/Ren followed you<\/a>.*?role="img" aria-label="Unread"/);
  expect(screen).toMatch(/<p class="[^"]*text-caption[^"]*"><span>No notifications yet\.<\/span><\/p>/);
});

test("the rail and the column beside the main one have no scrollbar of their own", () => {
  const side = /<nav class="(hidden tablet:sticky[^"]*)"/.exec(screen)?.[1];
  const aside = /<aside class="([^"]*)"/.exec(screen)?.[1];
  for (const classes of [side, aside]) {
    expect(classes).toBeDefined();
    expect(classes).not.toMatch(/overflow-y-auto|max-h-dvh|:h-dvh/);
  }
});

test("the mark is a link named by its label, its images decorative", () => {
  expect(screen).toMatch(/<a [^>]*aria-label="mycroblog"/);
  expect(screen.match(/<img [^>]*alt=""/g)).toHaveLength(2);
});

test("the rail, the tab bar and the column beside the main one show at their breakpoints", () => {
  expect(screen).toMatch(/<nav class="hidden tablet:sticky/);
  expect(screen).toMatch(
    /<div class="[^"]*tablet:hidden"><div [^>]*data-frame-call=""[^>]*>.*<nav [^>]*aria-label="Main"/,
  );
  expect(screen).toMatch(/<aside class="hidden [^"]*wide:grid/);
});
