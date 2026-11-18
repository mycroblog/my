// SPDX-License-Identifier: AGPL-3.0-only

// Relationships of the settings for its tests (#3202), on either DOM simulation: the entry, or the whole list under
// it, drawn on a fake server, the servers the tests read it on, the four lists with what undoing a row of each sends
// and says, and the parts a test finds on the screen.

import { instanceQuery, viewerQuery } from "@my/app-data";
import { scenario, type FakeOptions, type FakePerson } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import { RelationshipListScreen } from "../settings/relationships";
import { SettingsEntryScreen } from "../settings/screen";
import { renderFeature, type FakeData } from "./feature";
import { answersOutsideAct } from "./filter-form";
import { runtimeSignals } from "./profile-form";

/**
 * The scenario's server with two muted servers and two followed hashtags beside its blocks (quickdeals, and the
 * account sora left behind on another server) and its mute (kai).
 */
export const some: FakeOptions = {
  domainMutes: ["spam.example", "ads.example"],
  followedTags: ["baking", "photography"],
};

/** `count` more people of the server, whom the reader may block or mute: Person 1, Person 2, … */
const crowd = (count: number): FakePerson[] => {
  const model = scenario.people.find((person) => person.username === "sam");
  if (model === undefined) throw new Error("test: the scenario has no sam");
  return Array.from({ length: count }, (_, i) => ({
    ...model,
    username: `person${String(i + 1).padStart(2, "0")}`,
    displayName: `Person ${String(i + 1)}`,
    email: `person${String(i + 1)}@demo.test`,
  }));
};

/** A server where the reader blocks `blocked` people, mutes `muted` others and `servers` servers, and follows `hashtags` hashtags. */
export function serverOf({
  blocked = 0,
  muted = 0,
  servers = 0,
  hashtags = 0,
}: {
  blocked?: number;
  muted?: number;
  servers?: number;
  hashtags?: number;
}): FakeOptions {
  const others = crowd(blocked + muted);
  return {
    seed: {
      people: [...scenario.people, ...others],
      restrictions: others.map((person, i) => ({
        kind: i < blocked ? "block" : "mute",
        by: "yuki",
        target: person.username,
      })),
    },
    domainMutes: Array.from({ length: servers }, (_, i) => `server-${String(i + 1).padStart(2, "0")}.example`),
    followedTags: Array.from({ length: hashtags }, (_, i) => `tag${String(i + 1).padStart(2, "0")}`),
  };
}

/**
 * Draws Relationships on the server `on`, in the main landmark as the shell places it, once the shell read the
 * instance and the reader's account: the entry, or with `list` the screen of that whole list. `told` holds `back` for
 * each time the way back was taken.
 */
export async function relationships(
  { data }: FakeData,
  { list, locale = "en" }: { list?: string; locale?: string } = {},
): Promise<{ user: UserEvent; told: string[] }> {
  answersOutsideAct();
  runtimeSignals();
  await data.queryClient.query(instanceQuery(data));
  await data.queryClient.query(viewerQuery(data));
  const told: string[] = [];
  const user = renderFeature(
    data,
    <main>
      {list === undefined ? (
        <SettingsEntryScreen entry="relationships" onBack={() => told.push("back")} languages={["en", "ja"]} />
      ) : (
        <RelationshipListScreen list={list} onBack={() => told.push("back")} />
      )}
    </main>,
    locale,
  );
  return { user, told };
}

/** Waits until the screen has read what it reads: no request of the app's data is on its way. */
export const read = ({ data }: FakeData) =>
  waitFor(() => {
    if (data.queryClient.isFetching() !== 0) throw new Error("test: the screen is still reading");
  });

/** The marks that isolate a name's direction inside a sentence (U+2066 to U+2069), which no reader sees. */
const isolates = new RegExp(`[${String.fromCodePoint(0x2066)}-${String.fromCodePoint(0x2069)}]`, "g");

/** `text` as a reader reads it: without the marks that isolate a name's direction, each run of spaces as one. */
export const plain = (text: string | null | undefined): string =>
  (text ?? "").replace(isolates, "").replace(/\s+/g, " ").trim();

/** A pattern of `parts` in their order, with or without space between them: what separates them is the layout's. */
export const inOrder = (...parts: string[]) =>
  new RegExp(`^${parts.map((part) => part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("\\s*")}$`);

/** The four lists: the entry's sections in their order, and what undoing a row of each sends and says. */
export const lists = [
  {
    list: "blocked",
    name: "Blocked accounts",
    undo: "Unblock",
    route: "DELETE /accounts/{id}/block",
    done: "Unblocked",
    all: "See all blocked accounts",
  },
  {
    list: "muted",
    name: "Muted accounts",
    undo: "Unmute",
    route: "DELETE /accounts/{id}/mute",
    done: "Unmuted",
    all: "See all muted accounts",
  },
  {
    list: "servers",
    name: "Muted servers",
    undo: "Unmute",
    route: "DELETE /domain_mutes/{domain}",
    done: "Unmuted",
    all: "See all muted servers",
  },
  {
    list: "hashtags",
    name: "Followed hashtags",
    undo: "Unfollow",
    route: "DELETE /tags/{hashtag}/follow",
    done: "Unfollowed",
    all: "See all followed hashtags",
  },
] as const;

export type ListKind = (typeof lists)[number];

/**
 * The first row of each list on the server `some`: the name of the group that holds its button, the request that
 * undoes it (given the ID of an account), and the sentences of the undo done and of the undo that failed.
 */
export const first: Record<
  ListKind["list"],
  {
    group: string;
    /** What the row says of what it holds, part by part. */
    title: readonly string[];
    username?: string;
    path: (id: string) => string;
    said: string;
    failed: string;
  }
> = {
  blocked: {
    group: "@quickdeals",
    title: ["QuickDeals", "@quickdeals@demo.test"],
    username: "quickdeals",
    path: (id) => `DELETE /accounts/${id}/block`,
    said: "You unblocked @quickdeals.",
    failed: "Unblocking @quickdeals did not finish. Try again.",
  },
  muted: {
    group: "@kai",
    title: ["Kai", "@kai@demo.test"],
    username: "kai",
    path: (id) => `DELETE /accounts/${id}/mute`,
    said: "You unmuted @kai.",
    failed: "Unmuting @kai did not finish. Try again.",
  },
  servers: {
    group: "spam.example",
    title: ["spam.example"],
    path: () => "DELETE /domain_mutes/spam.example",
    said: "You unmuted the server spam.example.",
    failed: "Unmuting the server spam.example did not finish. Try again.",
  },
  hashtags: {
    group: "photography",
    title: ["photography"],
    path: () => "DELETE /tags/photography/follow",
    said: "You unfollowed #photography.",
    failed: "Unfollowing #photography did not finish. Try again.",
  },
};

/** The section of the entry named `name`, whose heading says its count after its name once it is known. */
export const section = (name: string) => screen.getByRole("region", { name: new RegExp(`^${name}(\\s*[\\d,]+)?$`) });
/** What the heading of the section named `name` says: its name, then its count. */
export const headed = (name: string) => plain(within(section(name)).getAllByRole("heading")[0]?.textContent);
/** The rows of the list of `part`: the list's own, not the row that leads to the whole list. */
export const rows = (part: HTMLElement) => {
  const [list] = within(part).queryAllByRole("list");
  return list === undefined ? [] : within(list).getAllByRole("listitem");
};
/** What the status regions under `part` say; a section keeps one from the start, empty while it has nothing to say. */
export const said = (part: HTMLElement = document.body) =>
  within(part)
    .queryAllByRole("status")
    .map((status) => plain(status.textContent))
    .filter((words) => words !== "");
/** The requests since `from` that write. */
export const writes = ({ requests }: FakeData, from = 0) => requests.slice(from).filter((r) => !r.startsWith("GET "));
