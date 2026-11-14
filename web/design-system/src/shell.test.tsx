// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import {
  Frame,
  FrameAction,
  FrameAside,
  FrameFloat,
  FrameHeading,
  FrameMain,
  FrameMark,
  FrameSection,
  FrameSide,
  FrameTabBar,
  NavItem,
  NavList,
} from "./shell";

const places = (
  <NavList>
    <NavItem href="/" icon="home" label="Home" current />
    <NavItem href="/notifications" icon="notifications" label="Notifications" />
  </NavList>
);

const screen = renderToStaticMarkup(
  <Frame>
    <FrameSide label="Main">
      <FrameMark href="/" label="mycroblog" light="/light.svg" dark="/dark.svg" />
      <FrameAction icon="compose" label="New post" onPress={() => undefined} aria-keyshortcuts="N" />
      {places}
    </FrameSide>
    <FrameMain>
      <FrameHeading>Home</FrameHeading>
    </FrameMain>
    <FrameAside label="Around you">
      <FrameSection id="server" icon="about" href="/about" heading="About this server">
        <p>my.example</p>
      </FrameSection>
    </FrameAside>
    <FrameTabBar label="Main">{places}</FrameTabBar>
    <FrameFloat>
      <FrameAction icon="compose" label="New post" onPress={() => undefined} />
    </FrameFloat>
  </Frame>,
);

test("the regions are landmarks with their names, and the screen's heading is the one h1", () => {
  expect(screen.match(/<nav [^>]*aria-label="Main"/g)).toHaveLength(2);
  expect(screen).toContain("<main ");
  expect(screen).toMatch(/<aside [^>]*aria-label="Around you"/);
  expect(screen.match(/<h1 /g)).toHaveLength(1);
  expect(screen).toMatch(/<h1 [^>]*tabindex="-1"[^>]*>Home<\/h1>/);
});

test("the place where the reader is is current, and each place is named by its label", () => {
  expect(screen.match(/aria-current="page"/g)).toHaveLength(2);
  expect(screen).toMatch(
    /<a [^>]*aria-current="page"[^>]*aria-label="Home"|<a [^>]*aria-label="Home"[^>]*aria-current="page"/,
  );
  expect(screen).toContain('aria-label="Notifications"');
});

test("the primary action is named by its label and carries its keys", () => {
  expect(screen).toMatch(
    /<button [^>]*aria-label="New post"[^>]*aria-keyshortcuts="N"|<button [^>]*aria-keyshortcuts="N"[^>]*aria-label="New post"/,
  );
});

test("a section is named by its heading, which links to its place", () => {
  expect(screen).toMatch(/<section [^>]*aria-labelledby="server"/);
  expect(screen).toMatch(/<h2 id="server"[^>]*><a [^>]*href="\/about"/);
});

test("the mark is a link named by its label, its images decorative", () => {
  expect(screen).toMatch(/<a [^>]*aria-label="mycroblog"/);
  expect(screen.match(/<img [^>]*alt=""/g)).toHaveLength(2);
});

test("the rail, the tab bar and the column beside the main one show at their breakpoints", () => {
  expect(screen).toMatch(/<nav class="hidden tablet:sticky/);
  expect(screen).toMatch(/<nav class="fixed [^"]*tablet:hidden"/);
  expect(screen).toMatch(/<aside class="hidden [^"]*wide:grid/);
});
