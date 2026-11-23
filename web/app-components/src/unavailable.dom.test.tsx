// SPDX-License-Identifier: AGPL-3.0-only

// An account the reader may not see (#3387): the server sends a stub marked `unavailable`, with no name, no avatar and
// no profile. Each line that shows one names it "Unavailable account" in the reader's language, with its domain on the
// handle's line, an empty avatar, no link to a profile and none of the row's actions; an event's line says it is by an
// unavailable account; and a report of an event whose organizer is one names no one, and offers neither mute nor block
// once sent. The words come from the package's catalog, which the copy role may change.

import { expect, test } from "bun:test";
import { cleanup, screen, within } from "@testing-library/react";
import { AccountLine, AccountName } from "./account";
import { EventListLine, type EventShown } from "./event-line";
import { stringsIn } from "./locale";
import { reportDraft, ReportPanel, type ReportAfter, type ReportTarget } from "./report";
import type { PostAccount } from "./status";
import { asked, inLocale, lena } from "./test/post";

/** An account the reader may not see, as the server sends it: its ID and its domain, nothing of its profile. */
const gone: PostAccount = { id: "gone", username: "", acct: "", domain: "far.example", display_name: "", emojis: [] };
const unavailable: PostAccount = { ...gone, unavailable: true };

const locales = ["en", "ja"] as const;

/** Every link of the document, by where it leads. */
const links = () => screen.queryAllByRole("link").map((link) => link.getAttribute("href"));

/** The row of `account`, with a Follow at its end, alone in the document for a reader of `locale`. */
function line(account: PostAccount, locale = "en") {
  cleanup();
  return inLocale(<AccountLine account={account} end={<button type="button">Follow</button>} />, locale);
}

for (const locale of locales)
  test(`an unavailable account's row is named so in ${locale}, with its domain, and with no link and no action`, () => {
    const { t } = stringsIn(locale);
    line(unavailable, locale);
    expect(screen.getByText(t("account.unavailable"))).toBeDefined();
    expect(screen.getByText("far.example")).toBeDefined();
    expect(links()).toEqual([]);
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });

test("an available account's row links to its profile and keeps its action, as the unavailable one does not", () => {
  line(lena);
  expect(links()).toEqual(["/@lena@toot.example.eu"]);
  expect(screen.getAllByRole("button", { name: "Follow" })).toHaveLength(1);
});

test("an unavailable account's row shows no avatar, even where the account carries one", () => {
  line({ ...unavailable, avatar: { small: { url: "https://far.example/gone.webp" } } });
  expect(asked("gone.webp")).toEqual([]);
  expect(screen.getByText(stringsIn("en").t("account.unavailable"))).toBeDefined();
});

for (const locale of locales)
  test(`an unavailable account's name reads as one in ${locale}`, () => {
    cleanup();
    const { container } = inLocale(<AccountName account={unavailable} />, locale);
    expect(container.textContent).toBe(stringsIn(locale).t("account.unavailable"));
  });

const now = Date.parse("2026-11-19T03:00:00Z");

/** An event of `organizer` two days on, as the list of the upcoming events shows it. */
const eventBy = (organizer: PostAccount): EventShown => ({
  id: "5",
  title: "Night market tour",
  organizer,
  time_zone: "Asia/Tokyo",
  starts_at: new Date(now + 2 * 86_400_000).toISOString(),
  ends_at: new Date(now + 2 * 86_400_000 + 7_200_000).toISOString(),
  state: "scheduled",
  places: [{ kind: "physical" }],
  hidden_places: [],
  going_count: 3,
});

for (const locale of locales)
  test(`an event's line by an unavailable account says so in ${locale}, and leads to the event alone`, () => {
    cleanup();
    inLocale(<EventListLine event={eventBy(unavailable)} now={now} />, locale);
    const [link] = screen.getAllByRole("link");
    if (link === undefined) throw new Error("test: the event's line has no link");
    expect(link.textContent).toContain(stringsIn(locale).t("event.by.unavailable"));
    expect(links()).toEqual(["/events/5"]);
  });

const ready: ReportAfter = { state: "ready", onPress: () => undefined };
const instance = "mycroblog demo";

/** The report of an event of `account`, alone in the document for a reader of `locale`, at `phase`. */
function report(account: ReportTarget["account"], phase: "writing" | "sent", locale = "en") {
  cleanup();
  inLocale(
    <ReportPanel
      target={{ kind: "event", account, name: "Night market tour" }}
      phase={phase}
      mute={ready}
      block={ready}
      instance={instance}
      rules={[]}
      draft={reportDraft()}
      onChange={() => undefined}
      onSend={() => undefined}
      onClose={() => undefined}
    />,
    locale,
  );
  return within(screen.getByRole("dialog"));
}

for (const locale of locales)
  test(`a report of an event whose organizer is unavailable says who reads it in ${locale}, naming no one`, () => {
    const dialog = report(unavailable, "writing", locale);
    expect(dialog.getByText(stringsIn(locale).t("report.intro.unavailable", { instance }))).toBeDefined();
    expect(screen.getByRole("dialog").textContent).not.toContain("far.example");
  });

test("once sent, a report of an event whose organizer is unavailable offers neither mute nor block", () => {
  // The same panel of an available organizer offers both: the queries find them where they are.
  const offered = report(lena, "sent");
  expect(offered.getAllByRole("button", { name: /^Mute / })).toHaveLength(1);
  expect(offered.getAllByRole("button", { name: /^Block / })).toHaveLength(1);

  const sent = report(unavailable, "sent");
  expect(sent.queryAllByRole("button", { name: /^(Mute|Block)/ })).toHaveLength(0);
  expect(sent.queryAllByRole("button", { name: /far\.example/ })).toHaveLength(0);
});
