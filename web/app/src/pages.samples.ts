// SPDX-License-Identifier: AGPL-3.0-only

// What the screens' stories add to the scenario so that a list the server pages holds more than one page (#3279):
// people of the server enough for a page and more, who follow, ask to follow, are a list's members, are blocked,
// react, go to an event or are invited to it, are held back, are listed supporters or are invited to a room; events
// and voice rooms enough. A story shows the first page under Show more, and the list after Show more pressed once
// (`more` of app.samples.tsx).

import { scenario, type FakeEventPlan, type FakeOptions, type FakePerson, type FakeSeed } from "@my/e2e/fake";

/** More items than a page of the server's holds (twenty), so that a list has a second page. */
const count = 24;

/** More who reacted than a page of theirs holds (forty, as the app asks). */
const reactors = 44;

const model = scenario.people.find((person) => person.username === "sam");
if (model === undefined) throw new Error("samples: the scenario has no sam");

/** People of the server, drawn as sam is: Person 01, Person 02, … */
const crowd: readonly FakePerson[] = Array.from({ length: reactors }, (_, i) => {
  const n = String(i + 1).padStart(2, "0");
  return { ...model, username: `person${n}`, displayName: `Person ${n}`, email: `person${n}@demo.test` };
});

/** The first `count` of the crowd. */
const some = crowd.slice(0, count);

/** The scenario with the crowd among its people, and `more` laid over it. */
const seeded = (more: Partial<FakeSeed> = {}): FakeOptions => ({
  seed: { ...scenario, people: [...scenario.people, ...crowd], ...more },
});

/** sam followed by the crowd too. */
export const manyFollowers: FakeOptions = seeded({
  follows: [...scenario.follows, ...some.map((p) => ({ follower: p.username, followee: "sam" }))],
});

/** ren, a locked account, asked by the crowd. */
export const manyFollowRequests: FakeOptions = {
  ...seeded(),
  followRequests: { from: some.map((p) => p.username) },
};

/** The reader's first list with the crowd among its members. */
export const manyMembers: FakeOptions = seeded({
  lists: scenario.lists.map((list, i) =>
    i === 0 ? { ...list, members: [...list.members, ...some.map((p) => p.username)] } : list,
  ),
});

/** The reader blocks the crowd's first too. */
export const manyBlocked: FakeOptions = seeded({
  restrictions: [
    ...scenario.restrictions,
    ...some.map((p) => ({ kind: "block" as const, by: "yuki", target: p.username })),
  ],
});

/** Who reacted with 🎉 to yuki's post of the ridge: the crowd too. */
export const manyReactors: FakeOptions = seeded({
  interactions: [
    ...scenario.interactions,
    ...crowd.map((p) => ({ kind: "reaction" as const, by: p.username, post: "yuki-ridge", emoji: "🎉" })),
  ],
});

/** The Sourdough class with the crowd going and invited. */
export const manyAttending: FakeOptions = seeded({
  events: scenario.events.map((plan) =>
    plan.key === "bread-class"
      ? {
          ...plan,
          capacity: 0,
          attendees: [...(plan.attendees ?? []), ...some.map((p) => ({ by: p.username, answer: "going" as const }))],
          invited: [...(plan.invited ?? []), ...some.map((p) => p.username)],
        }
      : plan,
  ),
});

/** Follows of the crowd held back too, beside the scenario's held notifications. */
export const manyHeld: FakeOptions = { ...seeded(), held: true, heldFrom: some.map((p) => p.username) };

/** The crowd listed as supporters too, the list shown to everyone. */
export const manySupporters: FakeOptions = {
  ...seeded(),
  supporters: { tips: {}, list: "everyone", more: some.map((p) => p.username) },
};

/** More open rooms and rooms the reader hosted than a page holds, and the crowd invited to the reader's room. */
export const manyVoice: FakeOptions = {
  ...seeded(),
  voiceMany: { rooms: count, history: count, invited: some.map((p) => p.username) },
};

/** Events of hana at noon in UTC, one a day from 30 days on, after the scenario's. */
export const manyEvents: FakeOptions = seeded({
  events: [
    ...scenario.events,
    ...Array.from({ length: count }, (_, i): FakeEventPlan => ({
      key: `later-${String(i)}`,
      organizer: "hana",
      title: `Open studio ${String(i + 1)}`,
      description: "",
      timeZone: "UTC",
      inDays: 30 + i,
      hour: 12,
      hours: 1,
    })),
  ],
});
