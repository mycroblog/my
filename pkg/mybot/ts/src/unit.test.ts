// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { missingScopes } from "./bot.js";
import { Budget, defaultPostRate, defaultReplyLimits, defaultWriteRate, ReplyLimiter } from "./budget.js";
import { replyIdempotencyKey, replyVisibility } from "./context.js";
import { Tracker } from "./dispatch.js";
import { FileStore } from "./filestore.js";
import { defaultHelp, parseCommand } from "./handler.js";
import { plainText } from "./text.js";
import { account } from "./test/fake.js";

describe("plainText", () => {
  test.each([
    ["", ""],
    ["<p>hello</p>", "hello"],
    [
      `<p><span class="h-card"><a href="https://example.com/@bot" class="u-url mention">@<span>bot</span></a></span> ping</p>`,
      "@bot ping",
    ],
    ["<p>one<br>two<br/>three</p><p>four</p>", "one\ntwo\nthree\n\nfour"],
    [
      `<p>see <a href="https://example.com/x"><span class="invisible">https://</span>example.com/x</a></p>`,
      "see https://example.com/x",
    ],
    ["<p>a &amp; b &lt;c&gt; &#39;d&#39; &#x1F600;</p>", "a & b <c> 'd' 😀"],
    ["<p>1 < 2</p>", "1 < 2"],
  ])("%p", (input, want) => {
    expect(plainText(input)).toBe(want);
  });
});

describe("parseCommand", () => {
  const me = { ...account(0, "bot", true), discoverable: false } as Parameters<typeof parseCommand>[1];
  test.each([
    ["@bot ping", { name: "ping", args: "" }],
    ["@Bot DICE 2d6  please ", { name: "dice", args: "2d6  please" }],
    ["@bot@example.com, help", { name: "help", args: "" }],
    ["@bot @bot hi there", { name: "hi", args: "there" }],
    ["@alice @bot ping", { name: "@alice", args: "@bot ping" }],
    ["ping", { name: "ping", args: "" }],
    ["@bot", null],
    ["   ", null],
  ])("%p", (input, want) => {
    expect(parseCommand(input, me)).toEqual(want);
  });
});

test("replyVisibility never widens and turns public into unlisted", () => {
  expect(replyVisibility("public")).toBe("unlisted");
  expect(replyVisibility("unlisted")).toBe("unlisted");
  expect(replyVisibility("local")).toBe("local");
  expect(replyVisibility("private")).toBe("private");
  expect(replyVisibility("direct")).toBe("direct");
});

test("replyIdempotencyKey is that of the Go library", () => {
  expect(replyIdempotencyKey("n1", 2)).toBe("mybot:reply:n1:2");
});

test("defaultHelp lists the commands with their descriptions", () => {
  expect(
    defaultHelp([
      { name: "help", description: "list the commands" },
      { name: "ping", description: "" },
    ]),
  ).toBe("help: list the commands\nping");
});

test("missingScopes counts the broad scopes", () => {
  const need = ["read:accounts", "read:notifications", "read:statuses", "write:statuses", "write:accounts"];
  expect(missingScopes("read:accounts read:notifications follow", need)).toEqual([
    "read:statuses",
    "write:statuses",
    "write:accounts",
  ]);
  expect(missingScopes("read write:statuses write:accounts", need)).toEqual([]);
  expect(missingScopes("follow", ["write:follows"])).toEqual([]);
});

describe("FileStore", () => {
  test("keeps the state across instances in a file of its owner, in the document of the Go library", async () => {
    const dir = await mkdtemp(join(tmpdir(), "mybot-"));
    try {
      const path = join(dir, "state.json");
      const s1 = new FileStore(path);
      expect(await s1.position()).toBeNull();
      await s1.markProcessed("a", Date.UTC(2026, 0, 1));
      await s1.markProcessed("b", Date.UTC(2026, 0, 3));
      await s1.setPosition("b");
      await s1.setLastRun("daily", Date.UTC(2026, 0, 2, 9));
      await s1.setRefreshToken("rt");
      await s1.forgetProcessed(Date.UTC(2026, 0, 2));
      expect((await stat(path)).mode & 0o777).toBe(0o600);

      const s2 = new FileStore(path);
      expect(await s2.isProcessed("a")).toBe(false);
      expect(await s2.isProcessed("b")).toBe(true);
      expect(await s2.position()).toBe("b");
      expect(await s2.lastRun("daily")).toBe(Date.UTC(2026, 0, 2, 9));
      expect(await s2.lastRun("weekly")).toBeNull();
      expect(await s2.refreshToken()).toBe("rt");
      expect(JSON.parse(await readFile(path, "utf8"))).toEqual({
        processed: { b: "2026-01-03T00:00:00.000Z" },
        position: "b",
        last_runs: { daily: "2026-01-02T09:00:00.000Z" },
        refresh_token: "rt",
      });

      // A file that the Go library wrote, with nanoseconds.
      await writeFile(path, `{"processed":{"c":"2026-01-04T00:00:00.123456789Z"},"position":"c"}\n`);
      const s3 = new FileStore(path);
      expect(await s3.isProcessed("c")).toBe(true);
      expect(await s3.position()).toBe("c");
      expect(await s3.refreshToken()).toBeNull();
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  test("a missing file is an empty state, and a broken one an error", async () => {
    const dir = await mkdtemp(join(tmpdir(), "mybot-"));
    try {
      expect(await new FileStore(join(dir, "none.json")).position()).toBeNull();
      const broken = join(dir, "broken.json");
      await writeFile(broken, "{");
      expect(new FileStore(broken).position()).rejects.toThrow(/read the state file/);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  });

  test("a change whose save fails leaves the store as it was", async () => {
    const root = await mkdtemp(join(tmpdir(), "mybot-"));
    try {
      const dir = join(root, "state");
      const s = new FileStore(join(dir, "state.json"));
      expect(s.markProcessed("lost", Date.now())).rejects.toThrow(/write the state file/);
      expect(await s.isProcessed("lost")).toBe(false);
      await mkdir(dir, { mode: 0o700 });
      await s.setPosition("p");
      const r = new FileStore(s.path);
      expect(await r.isProcessed("lost")).toBe(false);
      expect(await r.position()).toBe("p");
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
});

test("Tracker moves the position past the processed notifications older than every pending one", () => {
  const t = new Tracker();
  t.add("1");
  t.add("2");
  t.add("3");
  expect(t.complete("2")).toBeNull();
  expect(t.complete("1")).toBe("2");
  t.hold();
  t.add("4");
  expect(t.complete("3")).toBeNull();
  expect(t.complete("4")).toBeNull();
  expect(t.release()).toBe("4");
});

test("Budget delays writes, drops those that would wait too long, and follows the RateLimit fields", () => {
  const b = new Budget({ n: 2, per: 60_000 }, defaultWriteRate);
  const now = 1_000_000_000;
  expect(b.reserve("post", now, 60_000)).toBe(0);
  expect(b.reserve("post", now, 60_000)).toBe(0);
  expect(b.reserve("post", now, 60_000)).toBe(30_000);
  expect(b.reserve("post", now, 10_000)).toBeNull();

  // Less than a tenth of the server's limit left: wait for its reset.
  const h = new Headers({ "RateLimit-Policy": `"api";q=300;w=300`, RateLimit: `"api";r=29;t=120` });
  b.observe(h, now);
  expect(b.reserve("write", now, 600_000)).toBe(120_000);
  const b2 = new Budget(defaultPostRate, defaultWriteRate);
  b2.observe(new Headers({ "RateLimit-Policy": `"api";q=300;w=300`, RateLimit: `"api";r=30;t=120` }), now);
  expect(b2.reserve("write", now, 60_000)).toBe(0);
});

// The writes that wait for a pause of the server go out spaced by the budget after it, not all at its end.
test("Budget spaces the writes after a pause", () => {
  const b = new Budget(defaultPostRate, { n: 1, per: 1000 });
  const now = 1_000_000_000;
  b.observe(new Headers({ "RateLimit-Policy": `"api";q=300;w=300`, RateLimit: `"api";r=0;t=10` }), now);
  expect([0, 1, 2].map(() => b.reserve("write", now, 60_000))).toEqual([10_000, 11_000, 12_000]);
  expect(b.reserve("write", now, 12_000)).toBeNull();
  // After the pause, the bucket goes on from where the reservations left it.
  expect(b.reserve("write", now + 10_000, 60_000)).toBe(3000);
});

test("ReplyLimiter allows 5 replies in 10 minutes and 15 in an hour", () => {
  const l = new ReplyLimiter(defaultReplyLimits);
  const now = 1_000_000_000;
  const minute = 60_000;
  for (let i = 0; i < 5; i++) expect(l.take("alice", now + i * 1000)).toBe(true);
  expect(l.full("alice", now + minute)).toBe(true);
  expect(l.full("bob", now)).toBe(false);
  // Five more after 11 and after 22 minutes make 15 in the hour.
  for (const after of [11 * minute, 22 * minute]) {
    for (let i = 0; i < 5; i++) expect(l.take("alice", now + after + i * 1000)).toBe(true);
  }
  expect(l.full("alice", now + 40 * minute)).toBe(true);
  expect(l.full("alice", now + 61 * minute)).toBe(false);
});
