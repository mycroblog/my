// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { joinTabGroup, type TabGroup } from "./tabs";
import { installTabs } from "./test/tabs";

let restore = () => {};

afterEach(() => {
  restore();
});

function join(): TabGroup {
  const group = joinTabGroup("my:test");
  if (group === null) throw new Error("the browser of the test has no tab group");
  return group;
}

/** A lead that runs until the function it hands out is called. */
function leading(log: string[], tab: string) {
  let stop = () => {};
  const lead = (tookOver: boolean) => {
    log.push(`${tab} leads${tookOver ? " after another" : ""}`);
    return new Promise<void>((resolve) => {
      stop = resolve;
    });
  };
  return {
    lead,
    stop: () => {
      stop();
    },
  };
}

test("a browser without Web Locks or BroadcastChannel has no tab group", () => {
  ({ restore } = installTabs({ locks: false }));
  expect(joinTabGroup("my:test")).toBeNull();
  restore();
  ({ restore } = installTabs({ broadcast: false }));
  expect(joinTabGroup("my:test")).toBeNull();
});

test("one tab leads at a time, and the next takes over when it stops", async () => {
  ({ restore } = installTabs());
  const log: string[] = [];
  const [a, b] = [leading(log, "a"), leading(log, "b")];
  const ledA = join().lead(new AbortController().signal, a.lead);
  const ledB = join().lead(new AbortController().signal, b.lead);
  await Bun.sleep(0);
  expect(log).toEqual(["a leads"]);
  a.stop();
  await ledA;
  await Bun.sleep(0);
  expect(log).toEqual(["a leads", "b leads after another"]);
  b.stop();
  await ledB;
});

test("a tab that stops waiting never leads", async () => {
  ({ restore } = installTabs());
  const log: string[] = [];
  const [a, b] = [leading(log, "a"), leading(log, "b")];
  const ledA = join().lead(new AbortController().signal, a.lead);
  const waiting = new AbortController();
  const ledB = join().lead(waiting.signal, b.lead);
  await Bun.sleep(0);
  waiting.abort();
  await ledB;
  a.stop();
  await ledA;
  await Bun.sleep(0);
  expect(log).toEqual(["a leads"]);
});

test("a message reaches the other tabs of the group, not its sender, until they leave", () => {
  ({ restore } = installTabs());
  const [a, b, c] = [join(), join(), join()];
  const received: string[] = [];
  a.listen((m) => received.push(`a ${String(m)}`));
  const stopB = b.listen((m) => received.push(`b ${String(m)}`));
  c.listen((m) => received.push(`c ${String(m)}`));
  a.post("one");
  stopB();
  c.close();
  a.post("two");
  expect(received).toEqual(["b one", "c one"]);
});
