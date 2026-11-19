// SPDX-License-Identifier: AGPL-3.0-only

// What a host and a co-host may do with a participant (ADR-0100 §5, §6): the co-host what the host may, but name or
// remove co-hosts and hand the room over; nobody acts on the host or on themselves, and neither does a participant who
// neither hosts nor co-hosts; the host and a co-host do not remove each other; and the host of a room with others in
// it is the one whose leave asks who hosts next. The participants are the fake's, in the roles each case needs.

import { expect, test } from "bun:test";
import type { VoiceRoom, VoiceRoomParticipant } from "@my/app-data";
import { fakeData } from "../test/feature";
import { hostOperations, hostsOthers, mayActOn } from "./host";

/** The host-only operations of ADR-0100 §5 that a menu offers on a participant. */
const hostOnly = ["cohost", "uncohost", "hand-over"] as const;

/** The fake's room hosted by the reader, and its participants in the roles each test gives them. */
async function room(): Promise<{
  room: VoiceRoom;
  as: (index: number, role: Partial<VoiceRoomParticipant>) => VoiceRoomParticipant;
}> {
  const { fake, data } = fakeData();
  const { data: read } = await data.api.GET("/voice/rooms/{id}", {
    params: { path: { id: fake.voice.roomId(2) } },
  });
  if (read === undefined) throw new Error("test: the fake has no room 2");
  const people = read.participants;
  if (people.length < 5) throw new Error("test: the fake's room 2 has fewer than five participants");
  return {
    room: read,
    as: (index, role) => {
      const p = people[index];
      if (p === undefined) throw new Error(`test: no participant ${String(index)}`);
      return { ...p, role: "listener", hosting: false, cohosting: false, hand_raised: false, ...role };
    },
  };
}

test("a co-host is offered what the host is on a participant, but naming co-hosts and handing the room over", async () => {
  const { as } = await room();
  const host = as(0, { hosting: true, role: "speaker" });
  const cohost = as(1, { cohosting: true });
  for (const target of [as(2, {}), as(2, { hand_raised: true }), as(2, { role: "speaker" })]) {
    const byHost = hostOperations(host, target);
    expect(byHost).toContain("cohost");
    expect(byHost).toContain("hand-over");
    const byCohost = hostOperations(cohost, target);
    expect(byCohost.length).toBeGreaterThan(0);
    expect(byCohost.filter((op) => (hostOnly as readonly string[]).includes(op))).toEqual([]);
    expect([...byCohost].sort()).toEqual(byHost.filter((op) => !(hostOnly as readonly string[]).includes(op)).sort());
  }
});

test("the host is offered to remove a co-host from co-hosting, not to name them again", async () => {
  const { as } = await room();
  const ops = hostOperations(as(0, { hosting: true, role: "speaker" }), as(1, { cohosting: true }));
  expect(ops).toContain("uncohost");
  expect(ops).not.toContain("cohost");
  expect(ops).toContain("hand-over");
});

test("speaking: a listener is let speak, a raised hand also declined, a speaker made a listener", async () => {
  const { as } = await room();
  const host = as(0, { hosting: true, role: "speaker" });
  expect(hostOperations(host, as(2, {}))).toContain("grant");
  expect(hostOperations(host, as(2, {}))).not.toContain("dismiss");
  expect(hostOperations(host, as(2, { hand_raised: true }))).toContain("grant");
  expect(hostOperations(host, as(2, { hand_raised: true }))).toContain("dismiss");
  expect(hostOperations(host, as(2, { role: "speaker" }))).toContain("revoke");
  expect(hostOperations(host, as(2, { role: "speaker" }))).not.toContain("grant");
});

test("nobody acts on the host or on themselves", async () => {
  const { as } = await room();
  const host = as(0, { hosting: true, role: "speaker" });
  const cohost = as(1, { cohosting: true });
  for (const [self, target] of [
    [cohost, host],
    [host, host],
    [cohost, cohost],
  ] as const) {
    expect(hostOperations(self, target)).toEqual([]);
    expect(mayActOn(self, target)).toBe(false);
  }
});

test("a participant who neither hosts nor co-hosts acts on nobody", async () => {
  const { as } = await room();
  const others = [as(0, { hosting: true, role: "speaker" }), as(1, { cohosting: true }), as(3, {})];
  for (const self of [as(2, {}), as(2, { role: "speaker" })])
    for (const target of others) {
      expect(hostOperations(self, target)).toEqual([]);
      expect(mayActOn(self, target)).toBe(false);
    }
  expect(mayActOn(undefined, as(3, {}))).toBe(false);
});

test("the host and a co-host act on a participant; neither removes the host nor a co-host", async () => {
  const { as } = await room();
  const host = as(0, { hosting: true, role: "speaker" });
  const cohost = as(1, { cohosting: true });
  const other = as(4, { cohosting: true, role: "speaker" });
  expect(mayActOn(host, as(2, {}))).toBe(true);
  expect(mayActOn(cohost, as(2, {}))).toBe(true);
  expect(hostOperations(host, as(2, {}))).toContain("remove");
  expect(hostOperations(cohost, as(2, {}))).toContain("remove");
  expect(hostOperations(host, cohost)).not.toContain("remove");
  expect(hostOperations(cohost, other)).not.toContain("remove");
});

test("the host of a room with others in it hosts others; alone, or as a co-host, not", async () => {
  const { room: read, as } = await room();
  const host = as(0, { hosting: true, role: "speaker" });
  const cohost = as(1, { cohosting: true });
  const listener = as(2, {});
  const withOthers = { ...read, participants: [host, cohost, listener], participants_count: 3 };
  expect(hostsOthers(withOthers, host)).toBe(true);
  expect(hostsOthers(withOthers, cohost)).toBe(false);
  expect(hostsOthers(withOthers, listener)).toBe(false);
  expect(hostsOthers({ ...read, participants: [host], participants_count: 1 }, host)).toBe(false);
  expect(hostsOthers(withOthers, undefined)).toBe(false);
});
