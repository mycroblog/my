// SPDX-License-Identifier: AGPL-3.0-only

// What a conversation's line, head and reply are to a reader and to assistive technology: who it is with, whether it
// is unread, when its latest post was posted and where that leads, and which presses are the entry's.

import { expect, test } from "bun:test";
import { Button, Entry, Feed, FeedItem, IconButton, NavigationProvider } from "@my/design-system";
import { fireEvent, isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  ConversationAuthor,
  ConversationFaces,
  ConversationHead,
  ConversationLine,
  ConversationReply,
  ConversationText,
} from "./conversation";

const faces = ["Mika", "Ren", "Sam", "Lena"].map((name) => <span key={name}>{`Face of ${name}`}</span>);

const names = (
  <>
    <bdi>Mika</bdi>, <bdi>Ren</bdi> and 2 others
  </>
);

test("a conversation shows who it is with, its latest post in its language, and when that was posted", () => {
  render(
    <ConversationLine faces={faces} names={names} namesId="with" date="2026-11-16T09:00:00Z">
      <ConversationAuthor>You</ConversationAuthor>: <ConversationText lang="ja">こんにちは</ConversationText>
    </ConversationLine>,
  );

  expect(document.getElementById("with")?.textContent).toBe("Mika, Ren and 2 others");
  expect(screen.getByText("You").tagName).toBe("BDI");
  const text = screen.getByText("こんにちは");
  expect(text.lang).toBe("ja");
  expect(text.dir).toBe("auto");
  expect(screen.getByRole<HTMLTimeElement>("time").dateTime).toBe("2026-11-16T09:00:00.000Z");
  // Read, and with nowhere to lead: no mark, and its time is no link.
  expect(screen.queryByRole("img")).toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
});

test("the faces are the first three people's, and the names say them: a screen reader skips the faces", () => {
  render(
    <ConversationLine faces={faces} names={names} date="2026-11-16T09:00:00Z">
      <ConversationText>Hello</ConversationText>
    </ConversationLine>,
  );

  for (const name of ["Mika", "Ren", "Sam"]) expect(isInaccessible(screen.getByText(`Face of ${name}`))).toBe(true);
  expect(screen.queryByText("Face of Lena")).toBeNull();
});

test("an unread conversation has a mark that says so", () => {
  render(
    <ConversationLine faces={faces} names={names} date="2026-11-16T09:00:00Z" unread="Unread">
      <ConversationText>Hello</ConversationText>
    </ConversationLine>,
  );

  expect(screen.getByRole("img", { name: "Unread" })).not.toBeNull();
});

test("the time leads to the conversation, and a press on it or on more is theirs alone, never the entry's", async () => {
  const user = userEvent.setup();
  const went: string[] = [];
  const opened: string[] = [];
  let more = 0;
  render(
    <NavigationProvider navigate={(href) => went.push(href)}>
      <Feed label="Conversations" count={1} more={false} busy={false} onEntryClick={(key) => opened.push(key)}>
        <FeedItem id="c1" position={1}>
          <Entry labelledBy="with">
            <ConversationLine
              faces={faces}
              names={names}
              namesId="with"
              date="2026-11-16T09:00:00Z"
              href="/conversations/c1"
              more={<IconButton icon="more" label="More" onPress={() => more++} />}
            >
              <ConversationText>Hello</ConversationText>
            </ConversationLine>
          </Entry>
        </FeedItem>
      </Feed>
    </NavigationProvider>,
  );
  const entry = screen.getByRole("article", { name: /^Mika\s*, Ren and 2 others$/ });
  const time = within(entry).getByRole("link");
  expect(within(time).getByRole("time")).not.toBeNull();
  expect(time.getAttribute("href")).toBe("/conversations/c1");

  await user.click(time);
  await user.click(within(entry).getByRole("button", { name: "More" }));
  expect(went).toEqual(["/conversations/c1"]);
  expect(more).toBe(1);
  expect(opened).toEqual([]);

  await user.click(within(entry).getByText("Hello"));
  expect(opened).toEqual(["c1"]);
});

test("the head of a conversation is its back button, then the people's names as a heading, then more", () => {
  render(
    <ConversationHead back={<Button>Back</Button>} faces={faces} more={<IconButton icon="more" label="More" />}>
      <bdi>Mika</bdi> and <bdi>Ren</bdi>
    </ConversationHead>,
  );
  const back = screen.getByRole("button", { name: "Back" });
  const heading = screen.getByRole("heading", { level: 2, name: /^Mika\s+and\s+Ren$/ });
  const more = screen.getByRole("button", { name: "More" });

  expect(back.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(heading.compareDocumentPosition(more) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(isInaccessible(screen.getByText("Face of Mika"))).toBe(true);
});

test("the faces alone are kept from a screen reader, the first three of them", () => {
  render(<ConversationFaces>{faces}</ConversationFaces>);

  expect(isInaccessible(screen.getByText("Face of Sam"))).toBe(true);
  expect(screen.queryByText("Face of Lena")).toBeNull();
});

test("the reply is a form named by its label that says to whom it goes and how, and that nothing submits by itself", () => {
  render(
    <ConversationReply
      label="Your reply to Mika and Ren"
      to="To Mika and Ren"
      visibility="Direct"
      tools={<Button>Add images</Button>}
      count={<span>500 characters left</span>}
      action={<Button>Send</Button>}
    >
      <input aria-label="Text" />
    </ConversationReply>,
  );
  const form = screen.getByRole("form", { name: "Your reply to Mika and Ren" });
  const part = within(form);

  expect(part.getByText("To Mika and Ren")).not.toBeNull();
  expect(part.getByRole("img", { name: "Direct" })).not.toBeNull();
  for (const name of ["Add images", "Send"]) expect(part.getByRole("button", { name })).not.toBeNull();
  expect(part.getByText("500 characters left")).not.toBeNull();
  // Enter in a field of a form submits it: the form takes that submission and sends nothing.
  expect(fireEvent.submit(form)).toBe(false);
});
