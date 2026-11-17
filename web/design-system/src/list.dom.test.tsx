// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactNode } from "react";
import { I18nProvider } from "react-aria-components";
import { Button, TextButton } from "./button";
import {
  AccountRow,
  BackButton,
  ButtonPair,
  Excerpt,
  FacesRow,
  Fold,
  FoundRows,
  GroupHeading,
  ListTitle,
  NoticeCaption,
  NoticeLine,
  OpenRow,
  ResultGroup,
  RowItem,
  RowList,
  ScreenHead,
  ScreenStack,
  ScreenTitle,
  SentenceName,
  StateText,
  TabRow,
  UnreadMark,
} from "./list";

/** Whether `later` comes after `earlier` in the reading order. */
const follows = (later: Element, earlier: Element) =>
  (earlier.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

/** A page that follows no link: a click on one leaves the test's document where it is, as the app's router does. */
const Page = ({ children }: { children: ReactNode }) => (
  <div
    role="presentation"
    onClick={(event) => {
      event.preventDefault();
    }}
  >
    {children}
  </div>
);

const tabs = [
  { id: "all", name: "All" },
  { id: "mentions", name: "Mentions" },
  { id: "direct", name: "Direct" },
] as const;
type TabId = (typeof tabs)[number]["id"];

/** The tabs over the list of the current one, which their owner changes as they ask, and what they asked for. */
function Tabs({ asked, start = "all" }: { asked: TabId[]; start?: TabId }) {
  const [current, setCurrent] = useState<TabId>(start);
  return (
    <TabRow
      label="Show"
      tabs={tabs}
      current={current}
      onChange={(id) => {
        asked.push(id);
        setCurrent(id);
      }}
      end={<Button>Mark all as read</Button>}
    >
      <p>The list of {current}</p>
    </TabRow>
  );
}

const tab = (name: string) => screen.getByRole("tab", { name });

test("the tabs are a tablist named by their label, the current one selected, and the list under them its panel", () => {
  render(<Tabs asked={[]} start="mentions" />);

  const list = screen.getByRole("tablist", { name: "Show" });
  expect(
    within(list)
      .getAllByRole("tab")
      .map((each) => each.textContent),
  ).toEqual(["All", "Mentions", "Direct"]);
  expect(within(list).getByRole("tab", { selected: true }).textContent).toBe("Mentions");
  expect(screen.getByRole("tabpanel", { name: "Mentions" }).textContent).toBe("The list of mentions");
});

test("a press on a tab asks for it, and its owner's answer selects it and shows its list", async () => {
  const user = userEvent.setup();
  const asked: TabId[] = [];
  render(<Tabs asked={asked} />);

  await user.click(tab("Direct"));

  expect(asked).toEqual(["direct"]);
  expect(screen.getByRole("tab", { selected: true })).toBe(tab("Direct"));
  expect(screen.getByRole("tabpanel", { name: "Direct" }).textContent).toBe("The list of direct");
});

test("a tab row whose owner does not change the tab stays on the current one", async () => {
  const user = userEvent.setup();
  const asked: string[] = [];
  render(
    <TabRow label="Show" tabs={tabs} current="all" onChange={(id) => asked.push(id)}>
      <p>The list of all</p>
    </TabRow>,
  );

  await user.click(tab("Mentions"));

  expect(asked).toEqual(["mentions"]);
  expect(screen.getByRole("tab", { selected: true })).toBe(tab("All"));
});

test("Tab stops once in the tabs, on the current one, then on what ends their row, then on the panel", async () => {
  const user = userEvent.setup();
  render(<Tabs asked={[]} start="mentions" />);

  await user.tab();
  expect(document.activeElement).toBe(tab("Mentions"));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Mark all as read" }));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("tabpanel", { name: "Mentions" }));
});

test("the arrows move the focus along the tabs and select as they go, around the ends, and Home and End go to the ends", async () => {
  const user = userEvent.setup();
  const asked: TabId[] = [];
  render(<Tabs asked={asked} />);
  await user.tab();
  expect(document.activeElement).toBe(tab("All"));

  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(tab("Mentions"));
  expect(screen.getByRole("tab", { selected: true })).toBe(tab("Mentions"));
  await user.keyboard("{End}");
  expect(document.activeElement).toBe(tab("Direct"));
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(tab("All"));
  await user.keyboard("{ArrowLeft}");
  expect(document.activeElement).toBe(tab("Direct"));
  await user.keyboard("{Home}");
  expect(document.activeElement).toBe(tab("All"));

  expect(asked).toEqual(["mentions", "direct", "all", "direct", "all"]);
  expect(screen.getByRole("tabpanel", { name: "All" }).textContent).toBe("The list of all");
});

test("where the text runs right to left, the left arrow goes to the next tab", async () => {
  const user = userEvent.setup();
  const asked: TabId[] = [];
  render(
    <I18nProvider locale="ar-XB">
      <Tabs asked={asked} />
    </I18nProvider>,
  );
  await user.tab();

  await user.keyboard("{ArrowLeft}");
  expect(document.activeElement).toBe(tab("Mentions"));
  await user.keyboard("{ArrowRight}");
  expect(document.activeElement).toBe(tab("All"));

  expect(asked).toEqual(["mentions", "all"]);
});

test("a screen's stack says it is busy while what it holds loads, and not once it has", () => {
  const { rerender } = render(
    <ScreenStack busy>
      <p>The list</p>
    </ScreenStack>,
  );
  expect(screen.getByText("The list").closest("[aria-busy]")?.getAttribute("aria-busy")).toBe("true");

  rerender(
    <ScreenStack>
      <p>The list</p>
    </ScreenStack>,
  );
  expect(screen.getByText("The list").closest('[aria-busy="true"]')).toBeNull();
});

test("a group's heading is a heading, and a fold holds the button that shows more", async () => {
  const user = userEvent.setup();
  let shown = 0;
  render(
    <>
      <GroupHeading>Replies</GroupHeading>
      <Fold>
        <TextButton onPress={() => shown++}>Show 3 more replies</TextButton>
      </Fold>
    </>,
  );

  expect(screen.getByRole("heading", { level: 2, name: "Replies" })).not.toBeNull();
  await user.click(screen.getByRole("button", { name: "Show 3 more replies" }));
  expect(shown).toBe(1);
});

test("the unread mark is an image named by its label", () => {
  render(<UnreadMark label="Unread" />);

  expect(screen.getByRole("img", { name: "Unread" })).not.toBeNull();
});

test("a notice's line says its sentence and when it happened, with the unread mark only while unread", () => {
  const { rerender } = render(
    <NoticeLine icon="follow" date="2026-11-14T09:30:00Z" unread="Unread">
      <SentenceName>Lena</SentenceName> followed you
    </NoticeLine>,
  );
  expect(screen.getByText("followed you", { exact: false }).textContent).toBe("Lena followed you");
  expect(screen.getByRole("time").getAttribute("datetime")).toBe("2026-11-14T09:30:00.000Z");
  expect(screen.getByRole("img", { name: "Unread" })).not.toBeNull();
  // The line leads nowhere unless it is told where.
  expect(screen.queryByRole("link")).toBeNull();

  rerender(
    <NoticeLine icon="follow" date={new Date("2026-11-14T09:30:00Z")}>
      <SentenceName>Lena</SentenceName> followed you
    </NoticeLine>,
  );
  expect(screen.getByRole("time").getAttribute("datetime")).toBe("2026-11-14T09:30:00.000Z");
  expect(screen.queryByRole("img")).toBeNull();
});

test("a notice's line that leads somewhere is a link named by its sentence, and following it tells its owner", async () => {
  const user = userEvent.setup();
  let followed = 0;
  render(
    <Page>
      <NoticeLine
        icon="favourite"
        date="2026-11-14T09:30:00Z"
        unread="Unread"
        href="/@ren/status/1"
        onFollow={() => followed++}
      >
        Kiri favourited your post
      </NoticeLine>
    </Page>,
  );
  const link = screen.getByRole("link", { name: "Kiri favourited your post" });
  expect(link.getAttribute("href")).toBe("/@ren/status/1");

  await user.click(link);
  expect(followed).toBe(1);
});

test("a notice's sentence alone names the entry that is labelled by its id", () => {
  render(
    <article aria-labelledby="notice-1">
      <NoticeLine
        icon="react"
        emoji="🎉"
        avatar={<img alt="" src="/kiri.png" />}
        date="2026-11-14T09:30:00Z"
        id="notice-1"
        unread="Unread"
        href="/@ren/status/1"
        excerpt={<Excerpt lang="ja">おはようございます</Excerpt>}
        action={<Button>Dismiss</Button>}
      >
        Kiri reacted to your post
      </NoticeLine>
    </article>,
  );

  const entry = screen.getByRole("article", { name: "Kiri reacted to your post" });
  expect(within(entry).getByText("🎉")).not.toBeNull();
  expect(within(entry).getByText("おはようございます").lang).toBe("ja");
  expect(within(entry).getByRole("button", { name: "Dismiss" })).not.toBeNull();
});

test("a notice's caption says its sentence, then the unread mark, then its control", () => {
  const { rerender } = render(
    <NoticeCaption icon="reply" unread="Unread" action={<Button>Dismiss</Button>}>
      Ken mentioned you
    </NoticeCaption>,
  );
  const sentence = screen.getByText("Ken mentioned you");
  const mark = screen.getByRole("img", { name: "Unread" });
  expect(follows(mark, sentence)).toBe(true);
  expect(follows(screen.getByRole("button", { name: "Dismiss" }), mark)).toBe(true);

  rerender(<NoticeCaption icon="reply">Ken mentioned you</NoticeCaption>);
  expect(screen.getByText("Ken mentioned you")).not.toBeNull();
  expect(screen.queryByRole("img")).toBeNull();
});

test("a pair of buttons keeps its order: Tab reaches the refusal, then the confirmation", async () => {
  const user = userEvent.setup();
  render(
    <ButtonPair>
      <Button>Decline</Button>
      <Button kind="primary">Accept</Button>
    </ButtonPair>,
  );

  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Decline" }));
  await user.tab();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Accept" }));
});

test("what a field found is a group named by its label", () => {
  render(
    <FoundRows label="Accounts you can add">
      <AccountRow avatar={<img alt="" src="/lena.png" />} name="Lena" handle="@lena@my.example" />
    </FoundRows>,
  );

  const found = screen.getByRole("group", { name: "Accounts you can add" });
  expect(within(found).getByText("Lena")).not.toBeNull();
});

test("rows of one kind are a list named by its label, each row an item of it", () => {
  render(
    <RowList label="Members">
      <RowItem>
        <AccountRow avatar={<img alt="" src="/lena.png" />} name="Lena" handle="@lena@my.example" />
      </RowItem>
      <RowItem>
        <AccountRow avatar={<img alt="" src="/ken.png" />} name="Ken" handle="@ken@far.example" />
      </RowItem>
    </RowList>,
  );

  const items = within(screen.getByRole("list", { name: "Members" })).getAllByRole("listitem");
  expect(items).toHaveLength(2);
  expect(within(items[1] as HTMLElement).getByText("Ken")).not.toBeNull();
});

test("one kind of what a screen found is a region named by its heading", () => {
  render(
    <ResultGroup title="People">
      <p>No one by these words.</p>
    </ResultGroup>,
  );

  const group = screen.getByRole("region", { name: "People" });
  expect(within(group).getByRole("heading", { level: 2, name: "People" })).not.toBeNull();
  expect(within(group).getByText("No one by these words.")).not.toBeNull();
});

test("an account's row has its name leading to the profile, its handle left to right, its note, and what the reader may do last", async () => {
  const user = userEvent.setup();
  render(
    <AccountRow
      avatar={<img alt="" src="/lena.png" />}
      name="Lena"
      handle="@lena@my.example"
      href="/@lena"
      note="Asked to follow you"
      end={<Button>Accept</Button>}
    />,
  );

  const name = screen.getByRole("link", { name: "Lena" });
  expect(name.getAttribute("href")).toBe("/@lena");
  const handle = screen.getByText("@lena@my.example");
  expect(handle.dir).toBe("ltr");
  const note = screen.getByText("Asked to follow you");
  const accept = screen.getByRole("button", { name: "Accept" });
  expect(follows(handle, name)).toBe(true);
  expect(follows(accept, note)).toBe(true);
  // No heading unless the row heads a screen.
  expect(screen.queryByRole("heading")).toBeNull();

  await user.tab();
  expect(document.activeElement).toBe(name);
  await user.tab();
  expect(document.activeElement).toBe(accept);
});

test("an account's row without a place to lead to has its name as text", () => {
  render(<AccountRow avatar={<img alt="" src="/lena.png" />} name="Lena" handle="@lena@my.example" />);

  expect(screen.getByText("Lena")).not.toBeNull();
  expect(screen.queryByRole("link")).toBeNull();
});

test("a screen's head has its back button before the account's row, whose name is the screen's heading", async () => {
  const user = userEvent.setup();
  let back = 0;
  render(
    <ScreenHead back={<BackButton onPress={() => back++}>Notifications</BackButton>}>
      <AccountRow avatar={<img alt="" src="/lena.png" />} name="Lena" handle="@lena@my.example" href="/@lena" heading />
    </ScreenHead>,
  );

  const button = screen.getByRole("button", { name: "Notifications" });
  const heading = screen.getByRole("heading", { level: 2, name: "Lena" });
  expect(follows(heading, button)).toBe(true);
  expect(within(heading).getByRole("link", { name: "Lena" }).getAttribute("href")).toBe("/@lena");

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard("{Enter}");
  expect(back).toBe(1);
});

test("the row of what waits elsewhere is one link named by its title and hint", () => {
  render(<OpenRow href="/notifications/requests" icon="followers" title="Follow requests" hint="3 held back" />);

  const links = screen.getAllByRole("link");
  expect(links).toHaveLength(1);
  expect(screen.getByRole("link", { name: "Follow requests 3 held back" })).toBe(links[0] as HTMLElement);
  expect(links[0]?.getAttribute("href")).toBe("/notifications/requests");
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test("a row of faces is one link named by its label, which shows its count and reads no face", () => {
  render(
    <FacesRow
      href="/@ren/status/1/favourites"
      label="6 favourited"
      glyph="★"
      count="6"
      faces={[<img key="lena" alt="Lena" src="/lena.png" />, <img key="ken" alt="Ken" src="/ken.png" />]}
    />,
  );

  const link = screen.getByRole("link", { name: "6 favourited" });
  expect(link.getAttribute("href")).toBe("/@ren/status/1/favourites");
  expect(link.textContent).toContain("6");
  expect(screen.queryAllByRole("img")).toEqual([]);
});

test.failing("a row of faces says its label on hover", async () => {
  // Defect: FacesRowProps says the label shows on hover; the row has neither a title nor a tooltip.
  const user = userEvent.setup();
  render(<FacesRow href="/@ren/status/1/favourites" label="6 favourited" glyph="★" count="6" faces={[]} />);
  const link = screen.getByRole("link", { name: "6 favourited" });

  await user.hover(link);

  const told = link.title === "6 favourited" || screen.queryByRole("tooltip", { name: "6 favourited" }) !== null;
  expect(told).toBe(true);
});

test("a list's title is a heading of its glyph, its words and how many", () => {
  render(
    <ListTitle glyph="★" count="6">
      Favourited by
    </ListTitle>,
  );

  expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("★Favourited by6");
});

test("a screen's title is a heading of its name, after its back button and before what the reader may do", async () => {
  const user = userEvent.setup();
  let back = 0;
  render(
    <ScreenTitle
      glyph="hashtag"
      back={<BackButton onPress={() => back++}>Back</BackButton>}
      actions={<Button>Follow</Button>}
    >
      #baking
    </ScreenTitle>,
  );

  const button = screen.getByRole("button", { name: "Back" });
  const heading = screen.getByRole("heading", { level: 2, name: "#baking" });
  const follow = screen.getByRole("button", { name: "Follow" });
  expect(follows(heading, button)).toBe(true);
  expect(follows(follow, heading)).toBe(true);

  await user.tab();
  expect(document.activeElement).toBe(button);
  await user.keyboard(" ");
  expect(back).toBe(1);
  await user.tab();
  expect(document.activeElement).toBe(follow);
});

test("a screen's title reads a long name whole, and how many after the name", () => {
  const { rerender } = render(<ScreenTitle>#TheLongestHashtagAboutSlowMornings</ScreenTitle>);
  expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("#TheLongestHashtagAboutSlowMornings");

  rerender(
    <ScreenTitle glyph="list" count="3 members">
      Close friends
    </ScreenTitle>,
  );
  expect(screen.getByRole("heading", { level: 2, name: "Close friends 3 members" })).not.toBeNull();
});

test("what the reader did stands in the button's place as text", () => {
  render(<StateText>Accepted</StateText>);

  expect(screen.getByText("Accepted")).not.toBeNull();
  expect(screen.queryByRole("button")).toBeNull();
});
