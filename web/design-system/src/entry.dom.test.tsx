// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider, RouterProvider } from "react-aria-components";
import { Avatar } from "./avatar";
import { TextButton } from "./button";
import { ActionRow, Byline, CaptionLine, CaptionName, Embed, Entry, LivePart, Stack, Stamp } from "./entry";
import { IconButton } from "./icon-button";

/** Whether `later` comes after `earlier` in the reading order. */
const follows = (later: Element, earlier: Element) =>
  (earlier.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

test("an entry is an article named by what labels it, in its language, and a stop of Tab before what is inside it", async () => {
  const user = userEvent.setup();
  render(
    <Entry lang="ja" labelledBy="author">
      <span id="author">Sara</span>
      <p>おはようございます</p>
      <ActionRow>
        <IconButton icon="reply" label="Reply" />
      </ActionRow>
    </Entry>,
  );
  const entry = screen.getByRole("article", { name: "Sara" });
  expect(entry.lang).toBe("ja");
  // Outside a feed it states no position.
  expect(entry.hasAttribute("aria-posinset")).toBe(false);
  expect(entry.hasAttribute("aria-setsize")).toBe(false);

  await user.tab();
  expect(document.activeElement).toBe(entry);
  await user.tab();
  expect(document.activeElement).toBe(within(entry).getByRole("button", { name: "Reply" }));
});

test("an entry is in the reader's direction, whatever the language of what it holds", () => {
  render(
    <I18nProvider locale="ar-XB">
      <Entry lang="en">A post in English</Entry>
    </I18nProvider>,
  );

  const entry = screen.getByRole("article");
  expect(entry.dir).toBe("rtl");
  expect(entry.lang).toBe("en");
});

test("a byline's name leads to the profile and names the entry, before what ends its line and the handle, left to right", async () => {
  const user = userEvent.setup();
  const went: string[] = [];
  render(
    <RouterProvider navigate={(href) => went.push(href)}>
      <Entry labelledBy="name">
        <Byline
          avatar={<Avatar name="سارا" src="/sara.png" size="post" decorative />}
          name="سارا"
          nameId="name"
          handle="@sara@example.social"
          href="/@sara"
          end={<Stamp date="2025-12-20T09:41:00Z" />}
        />
      </Entry>
    </RouterProvider>,
  );

  const entry = screen.getByRole("article", { name: "سارا" });
  const name = within(entry).getByRole("link", { name: "سارا" });
  expect(name.getAttribute("href")).toBe("/@sara");
  const handle = within(entry).getByText("@sara@example.social");
  expect(handle.dir).toBe("ltr");
  const time = within(entry).getByRole("time");
  expect(follows(time, name)).toBe(true);
  expect(follows(handle, time)).toBe(true);
  // The avatar beside the name says nothing more.
  expect(within(entry).queryByRole("img")).toBeNull();
  expect(within(entry).queryByRole("heading")).toBeNull();

  await user.click(name);
  expect(went).toEqual(["/@sara"]);
});

test("a byline without a profile to lead to has its name as text, and one that heads a screen has it as the heading", () => {
  render(
    <>
      <Byline avatar={null} name="Ravi" handle="@ravi@x.example" />
      <Byline avatar={null} name="Mika" handle="@mika@x.example" heading />
    </>,
  );

  expect(screen.getByText("Ravi")).not.toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
  expect(screen.getByRole("heading", { level: 2, name: "Mika" })).not.toBeNull();
});

test("a byline of the reader's own says so, and the handle after it", () => {
  render(<Byline compact avatar={null} name="Ravi" handle="@ravi@x.example" self="You" />);

  const self = screen.getByText("You");
  const handle = screen.getByText("@ravi@x.example");
  expect(follows(handle, self)).toBe(true);
  expect(follows(self, screen.getByText("Ravi"))).toBe(true);
});

test("a stamp's glyph is an image named by what it says, before the time with its moment", () => {
  const { rerender } = render(<Stamp glyph={{ icon: "public", label: "Public" }} date="2025-12-20T09:41:00Z" />);
  const glyph = screen.getByRole("img", { name: "Public" });
  const time = screen.getByRole("time");
  expect(time.getAttribute("datetime")).toBe("2025-12-20T09:41:00.000Z");
  expect(follows(time, glyph)).toBe(true);
  // The time leads nowhere unless it is told where.
  expect(screen.queryByRole("link")).toBeNull();
  expect(screen.queryByRole("button")).toBeNull();

  rerender(<Stamp date={new Date("2025-12-20T09:41:00Z")} />);
  expect(screen.queryByRole("img")).toBeNull();
  expect(screen.getByRole("time").getAttribute("datetime")).toBe("2025-12-20T09:41:00.000Z");
});

test("a stamp's time leads to the entry's own page, and an edited entry has a button that opens its history", async () => {
  const user = userEvent.setup();
  const went: string[] = [];
  let opened = 0;
  render(
    <RouterProvider navigate={(href) => went.push(href)}>
      <I18nProvider locale="en">
        <Stamp
          glyph={{ icon: "public", label: "Public" }}
          date="2025-12-20T09:41:00Z"
          href="/@sara/statuses/1"
          edited={{ text: "edited", label: "Edited: show the history", onPress: () => opened++ }}
        />
      </I18nProvider>
    </RouterProvider>,
  );

  const link = screen.getByRole("link", { name: "Dec 20, 2025" });
  expect(link.getAttribute("href")).toBe("/@sara/statuses/1");
  expect(within(link).getByRole("time")).not.toBeNull();
  const edited = screen.getByRole("button", { name: "Edited: show the history" });
  expect(edited.textContent).toBe("edited");

  await user.tab();
  expect(document.activeElement).toBe(link);
  await user.keyboard("{Enter}");
  expect(went).toEqual(["/@sara/statuses/1"]);
  await user.tab();
  expect(document.activeElement).toBe(edited);
  await user.keyboard("{Enter}");
  expect(opened).toBe(1);
});

test("a caption line says its text in its language, and its action after a middle dot", async () => {
  const user = userEvent.setup();
  let shown = 0;
  render(
    <CaptionLine icon="translate" lang="en" action={<TextButton onPress={() => shown++}>Show original</TextButton>}>
      Translated from Japanese
    </CaptionLine>,
  );

  const line = screen.getByText("Translated from Japanese", { exact: false }).closest("[lang]");
  expect(line?.getAttribute("lang")).toBe("en");
  expect(line?.textContent).toBe("Translated from Japanese · Show original");
  await user.click(screen.getByRole("button", { name: "Show original" }));
  expect(shown).toBe(1);
});

test("a caption line that is its action alone, or that ends with one apart from its text, has no middle dot", () => {
  render(
    <>
      <CaptionLine icon="translate" action={<TextButton>Translate</TextButton>} />
      <CaptionLine icon="schedule" end={<TextButton>Change time</TextButton>}>
        Scheduled for tomorrow
      </CaptionLine>
    </>,
  );

  expect(screen.getByRole("button", { name: "Translate" })).not.toBeNull();
  expect(follows(screen.getByRole("button", { name: "Change time" }), screen.getByText("Scheduled for tomorrow"))).toBe(
    true,
  );
  expect(document.body.textContent).not.toContain("·");
});

test("a caption line that changes too often to be heard is read as what it is given to say", () => {
  render(
    <CaptionLine icon="translate" busy spoken="Downloading the language, half done">
      Downloading the language… 52%
    </CaptionLine>,
  );

  expect(isInaccessible(screen.getByText("Downloading the language… 52%"))).toBe(true);
  expect(isInaccessible(screen.getByText("Downloading the language, half done"))).toBe(false);
});

test("a one-line caption keeps the words around its name, with their spaces", () => {
  render(
    <CaptionLine icon="boost" oneLine>
      <CaptionName>A very long name</CaptionName> boosted
    </CaptionLine>,
  );

  expect(screen.getByText("A very long name").parentElement?.textContent).toBe("A very long name boosted");
});

test("a live part is one polite live region that stays while what is in it changes", () => {
  const { rerender } = render(
    <LivePart>
      <CaptionLine icon="translate">Translating…</CaptionLine>
    </LivePart>,
  );
  const region = screen.getByText("Translating…").closest("[aria-live]");
  expect(region?.getAttribute("aria-live")).toBe("polite");

  rerender(
    <LivePart>
      <CaptionLine icon="translate">Translated from Japanese</CaptionLine>
    </LivePart>,
  );
  expect(screen.getByText("Translated from Japanese").closest("[aria-live]")).toBe(region ?? null);
});

test("an action row keeps its order: Tab reaches its buttons, then what ends it", async () => {
  const user = userEvent.setup();
  render(
    <ActionRow lang="en" end={<IconButton icon="more" label="More" />}>
      <IconButton icon="reply" label="Reply" />
      <IconButton icon="boost" label="Boost" pressed={false} />
    </ActionRow>,
  );
  expect(screen.getByRole("button", { name: "Reply" }).closest("[lang]")?.getAttribute("lang")).toBe("en");

  for (const name of ["Reply", "Boost", "More"]) {
    await user.tab();
    expect(document.activeElement).toBe(screen.getByRole("button", { name }));
  }
});

test("a stack and an embed mark what they hold with its language", () => {
  render(
    <>
      <Stack gap="line" lang="de">
        <p>Guten Morgen</p>
      </Stack>
      <Embed lang="ja">
        <p>おはよう</p>
      </Embed>
      <Stack>
        <p>Good morning</p>
      </Stack>
    </>,
  );

  expect(screen.getByText("Guten Morgen").closest("[lang]")?.getAttribute("lang")).toBe("de");
  expect(screen.getByText("おはよう").closest("[lang]")?.getAttribute("lang")).toBe("ja");
  expect(screen.getByText("Good morning").closest("[lang]")).toBeNull();
});
