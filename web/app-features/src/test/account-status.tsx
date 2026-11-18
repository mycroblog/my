// SPDX-License-Identifier: AGPL-3.0-only

// The account's status in the settings (#3204), for its tests on either DOM simulation: the actions of the moderators
// a test seeds its server with, at moments that read the same on any day; the three screens drawn alone on a fake
// server, the entry Account and data, Account status under it, and one action with its appeal; and how a test reads
// what they say.

import { onTestFinished } from "bun:test";
import { instanceQuery, viewerQuery } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import type { components } from "@mycroblog/mykit";
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { ModerationActionScreen, type ModerationActionProps } from "../settings/account-action";
import { AccountStatusScreen } from "../settings/account-status";
import { SettingsEntryScreen } from "../settings/screen";
import { fakeData, renderFeature, type FakeData } from "./feature";
import { runtimeSignals } from "./profile-form";

/** An action of the moderators as a fake starts with it. */
export type Seed = NonNullable<NonNullable<FakeOptions["settings"]>["moderation"]>[number];

// The moments of the seeds are dates, at noon UTC so that they are the same day in the zone a suite runs in, and each
// far enough from today that what is to come stays to come: an action in effect ends in 2031.

/** An action that keeps the account from posting, in effect, which can be appealed until March 9, 2031. */
export const noPosting: Seed = {
  kind: "restrict_posting",
  reason_category: "spam",
  public_reason: "The same link was posted to forty hashtags within an hour.",
  state: "active",
  issued_at: "2024-11-02T12:00:00.000Z",
  expires_at: "2031-05-20T12:00:00.000Z",
  appealable: true,
  appeal_deadline: "2031-03-09T12:00:00.000Z",
};

/** A limit in effect with no end, whose appeal waits for the moderators. */
export const limitWaiting: Seed = {
  kind: "limit",
  reason_category: "violation",
  public_reason: "Repeated replies that took other people's threads off their subject.",
  state: "active",
  issued_at: "2024-10-20T12:00:00.000Z",
  appealable: false,
  appeal_deadline: "2031-04-18T12:00:00.000Z",
  appeal: {
    id: "waiting",
    text: "I replied to three threads that day, each on its subject. Please look at the replies again.",
    state: "pending",
    submitted_at: "2024-10-21T12:00:00.000Z",
  },
};

/** An action in effect that was not appealed, whose time to appeal passed on June 1, 2024. */
export const sensitivePassed: Seed = {
  kind: "force_sensitive",
  reason_category: "violation",
  public_reason: "Photos of injuries were posted without a content warning several times.",
  state: "active",
  issued_at: "2023-12-04T12:00:00.000Z",
  appealable: false,
  appeal_deadline: "2024-06-01T12:00:00.000Z",
};

/** A restriction that ended, which the moderators kept when it was appealed, with their answer. */
export const interactionUpheld: Seed = {
  kind: "restrict_interaction",
  reason_category: "violation",
  public_reason: "Following and unfollowing hundreds of accounts a day.",
  state: "expired",
  issued_at: "2024-02-01T12:00:00.000Z",
  expires_at: "2024-02-15T12:00:00.000Z",
  appealable: false,
  appeal_deadline: "2024-07-30T12:00:00.000Z",
  appeal: {
    id: "upheld",
    text: "I was looking for people who post about baking and went too fast. I have stopped.",
    state: "upheld",
    submitted_at: "2024-02-02T12:00:00.000Z",
    decided_at: "2024-02-04T12:00:00.000Z",
    decision_reason: "The follows continued after the first warning, so the restriction stays until it ends.",
  },
};

/** An action on posts, lifted when its appeal was decided. */
export const postsOverturned: Seed = {
  kind: "mark_statuses_sensitive",
  reason_category: "other",
  public_reason: "Photos of a medical procedure without a content warning.",
  state: "lifted",
  issued_at: "2024-01-10T12:00:00.000Z",
  lifted_at: "2024-01-18T12:00:00.000Z",
  appealable: false,
  appeal: {
    id: "overturned",
    text: "The photos are of my own recovery and I added a warning to each.",
    state: "overturned",
    submitted_at: "2024-01-12T12:00:00.000Z",
    decided_at: "2024-01-18T12:00:00.000Z",
    decision_reason: "On a second look the posts carried the warning. The action is lifted.",
  },
};

/** A warning that ended, with no explanation, never appealed, whose time to appeal has passed. */
export const warning: Seed = {
  kind: "warn",
  reason_category: "violation",
  state: "expired",
  issued_at: "2023-03-01T12:00:00.000Z",
  expires_at: "2023-03-02T12:00:00.000Z",
  appealable: false,
  appeal_deadline: "2023-08-28T12:00:00.000Z",
};

type ActionPage = components["schemas"]["ModerationActionPage"];

/** Words no screen may show: each stands for something of an action that only the moderators read. */
export const moderatorsOnly = /PRIVATE-NOTE|REPORT-ID|REPORTER|MODERATOR/;

/**
 * A page of the reader's actions as a server that broke its word would send it: each action with a private note, the
 * report it follows from, who filed that report and who took the action. A change for `GET /moderation/actions`.
 */
export const withTheModerators = (page: ActionPage): ActionPage => ({
  ...page,
  items: page.items.map(
    (action) =>
      // No member of the specification holds any of these, so the type of an action has no place for them.
      ({
        ...action,
        private_note: "PRIVATE-NOTE",
        report_id: "REPORT-ID",
        reporter: { acct: "REPORTER" },
        issued_by: { acct: "MODERATOR" },
      }) as typeof action,
  ),
});

/** A server whose reader has the actions `moderation`, newest first. */
export const acted = (...moderation: Seed[]) => ({ settings: { moderation } }) satisfies FakeOptions;

/** `count` warnings that ended, each with a reason that names its place from the newest (1) on. */
export const warnings = (count: number): Seed[] =>
  Array.from({ length: count }, (_, i) => ({ ...warning, public_reason: `Warning number ${String(i + 1)}.` }));

/** A screen drawn: its server, the reader's hands, and what the screen told its owner, in order. */
export type Drawn = FakeData & {
  user: UserEvent;
  /** `back` for the way back pressed, `unsaved:true|false` for each report of an appeal written and not sent, `stay`, `leave`. */
  told: string[];
};

/**
 * Draws `node` on `on` in the main landmark, as the shell places a screen, once the shell read the instance and the
 * reader's account. The server's answers reach the screen when they come, not inside an act() of the test, so React is
 * told so until the test ends, and a test waits for what it asserts.
 */
async function drawn(on: FakeData, node: (told: string[]) => ReactNode, locale?: string): Promise<Drawn> {
  const environment = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean };
  const wrapped = environment.IS_REACT_ACT_ENVIRONMENT;
  environment.IS_REACT_ACT_ENVIRONMENT = false;
  onTestFinished(() => {
    if (wrapped === undefined) delete environment.IS_REACT_ACT_ENVIRONMENT;
    else environment.IS_REACT_ACT_ENVIRONMENT = wrapped;
  });
  await on.data.queryClient.query(instanceQuery(on.data));
  await on.data.queryClient.query(viewerQuery(on.data));
  const told: string[] = [];
  const user = renderFeature(on.data, <main>{node(told)}</main>, locale);
  return { ...on, user, told };
}

/** How a screen is drawn besides its server. */
export type Drawing = {
  /** The language the screen reads in; `en` by default. */
  locale?: string;
};

/**
 * A server of `options` to draw a screen on, for a test that scripts an answer before the screen reads it
 * (`on.fake.script(…)`, then `drawnEntry(on)`).
 */
export function serverOf(options: FakeOptions = {}): FakeData {
  runtimeSignals();
  return fakeData(options);
}

/** Where a screen is drawn: on a new server of the options given, or on the server a test made with `serverOf`. */
export type Server = FakeOptions | FakeData;

const made = (from: Server): FakeData => ("fake" in from ? from : serverOf(from));

/** Draws `node`, a part of the settings that reads the actions when it is drawn. */
export const drawnPart = (
  from: Server,
  node: (told: string[]) => ReactNode,
  { locale }: Drawing = {},
): Promise<Drawn> => drawn(made(from), node, locale);

/** Draws the entry Account and data, which reads the actions when it is drawn. */
export const drawnEntry = (from: Server = {}, drawing: Drawing = {}): Promise<Drawn> =>
  drawnPart(
    from,
    (told) => <SettingsEntryScreen entry="account" onBack={() => told.push("back")} languages={["en", "ja"]} />,
    drawing,
  );

/** Draws Account status, which reads the actions when it is drawn. */
export const drawnStatus = (from: Server = {}, drawing: Drawing = {}): Promise<Drawn> =>
  drawnPart(from, (told) => <AccountStatusScreen onBack={() => told.push("back")} />, drawing);

/** The ID of the reader's action at `index`, newest first, from the app's address of it. */
export const actionId = ({ fake }: Pick<FakeData, "fake">, index: number): string =>
  fake.actionPath(index).split("/").at(-1) ?? "";

/**
 * Draws the screen of the reader's action at `index`, newest first, or of the address's `id` when given; `leaving`
 * draws it as the app does while the reader is asked whether to leave with an appeal written and not sent.
 */
export function drawnAction(
  from: Server = {},
  { index = 0, id, leaving = false, ...drawing }: Drawing & { index?: number; id?: string; leaving?: boolean } = {},
): Promise<Drawn> {
  const on = made(from);
  return drawn(
    on,
    (told) => {
      const asked: ModerationActionProps["leaving"] = {
        onStay: () => told.push("stay"),
        onLeave: () => told.push("leave"),
      };
      return (
        <ModerationActionScreen
          id={id ?? actionId(on, index)}
          onBack={() => told.push("back")}
          onUnsaved={(unsaved) => told.push(`unsaved:${String(unsaved)}`)}
          leaving={leaving ? asked : undefined}
        />
      );
    },
    drawing.locale,
  );
}

/**
 * What `element` says as a reader reads its line: each run of spaces, breaking or not, as one space. A date the screens
 * write may keep its parts together with spaces that do not break.
 */
export const wordsOf = (element: Element): string => element.textContent.replace(/\s+/g, " ").trim();

/** The screen's own part. */
export const main = () => within(screen.getByRole("main"));

/** What the paragraphs inside `part` say, in their order, the empty ones left out: its lines. */
export const linesOf = (part: HTMLElement): string[] =>
  within(part)
    .queryAllByRole("paragraph")
    .map(wordsOf)
    .filter((line) => line !== "");

/** The section of the screen whose heading begins with `name`: a heading may end with how many things it holds. */
export const section = (name: string): HTMLElement => {
  const found = main()
    .queryAllByRole("region")
    .filter((part) => wordsOf(within(part).getAllByRole("heading")[0] ?? part).startsWith(name));
  const [one, ...others] = found;
  if (one === undefined || others.length > 0)
    throw new Error(`test: ${String(found.length)} sections are titled ${name}`);
  return one;
};

/** The titles of the screen's sections whose heading begins with `name`. */
export const sectionsTitled = (name: string): string[] =>
  main()
    .queryAllByRole("region")
    .map((part) => wordsOf(within(part).getAllByRole("heading")[0] ?? part))
    .filter((title) => title.startsWith(name));

/** What the status regions of the screen say, the ones with nothing to say left out. */
export const said = (): string[] =>
  main()
    .queryAllByRole("status")
    .map(wordsOf)
    .filter((words) => words !== "");

/** What the alerts of the screen say. */
export const alerts = (): string[] => main().queryAllByRole("alert").map(wordsOf);

/** What describes `element`, part by part. */
export const describedBy = (element: HTMLElement): string[] =>
  (element.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => wordsOf(document.getElementById(id) ?? element));

/** The field of the appeal. */
export const appealField = () => main().getByRole<HTMLTextAreaElement>("textbox", { name: "Your appeal" });
/** The fields of the appeal the screen shows: one while the action can be appealed, none after. */
export const appealFields = () => main().queryAllByRole<HTMLTextAreaElement>("textbox", { name: "Your appeal" });
/** The button that sends the appeal. */
export const sendButton = () => main().getByRole<HTMLButtonElement>("button", { name: "Send appeal" });

/** Writes `text` in the appeal's field after what it holds, as one paste: no key decides anything here. */
export async function write(user: UserEvent, text: string): Promise<void> {
  await user.click(appealField());
  await user.paste(text);
}

/** What the whole screen says. */
export const screenWords = (): string => wordsOf(screen.getByRole("main"));

/**
 * Waits until the server answered a read of the actions sent since the request numbered `from` (the first by
 * default), nothing of the app's data is on its way, and the screen drew what came: an absence asserted after it is
 * one the answer left.
 */
export async function answered({ data, requests }: Pick<FakeData, "data" | "requests">, from = 0): Promise<void> {
  await waitFor(() => {
    if (!requests.slice(from).some((request) => request.startsWith("GET /moderation/actions")))
      throw new Error("test: the actions were not asked for yet");
    if (data.queryClient.isFetching() > 0) throw new Error("test: the screen is still reading");
  });
  // The screen draws what its data holds in a turn of its own.
  await new Promise((resolve) => setTimeout(resolve, 0));
}
