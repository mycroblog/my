// SPDX-License-Identifier: MIT
// Loads the ESM build of @mycroblog/mybot (dist/, and @mycroblog/mykit's through node_modules) on the runtime that
// runs this file, and runs a bot with a FileStore against a fake server in memory: the CI test that the package works
// on Node.js and Deno as it is published (ADR-0094 §1; Bun runs the unit tests). It exits non-zero on a failure.
//
//   node test/runtimes.mjs
//   deno run --allow-read --allow-write --allow-env test/runtimes.mjs
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Bot, FileStore, plainText } from "../dist/index.js";
import { staticToken } from "@mycroblog/mykit";

const runtime = typeof globalThis.Deno === "object" ? `Deno ${globalThis.Deno.version.deno}` : `Node.js ${globalThis.process.version}`;

const notificationId = "01900000-0000-7000-8000-000000000010";
const alice = { id: "01900000-0000-7000-8000-000000000001", username: "alice", acct: "alice", bot: false };
const mention = {
  id: notificationId,
  type: "mention",
  created_at: new Date().toISOString(),
  account: alice,
  status: { id: "01900000-0000-7000-8000-000000000510", account: alice, content: "<p>@bot ping</p>", visibility: "public" },
};
const posts = [];

/** The part of the client API that the bot uses. */
async function fakeFetch(request) {
  const url = new URL(request.url);
  const json = (body) => new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" } });
  switch (`${request.method} ${url.pathname}`) {
    case "GET /api/v1/accounts/verify_credentials":
      return json({ id: "01900000-0000-7000-8000-000000000000", username: "bot", acct: "bot", domain: "example.com", bot: true });
    case "GET /api/v1/notifications":
      return json({ items: [mention] });
    case "POST /api/v1/statuses":
      posts.push({ key: request.headers.get("Idempotency-Key"), body: await request.json() });
      return json({ id: "01900000-0000-7000-8000-000000009001", content: "", visibility: "unlisted" });
    default:
      return new Response(JSON.stringify({ type: "about:blank", title: "not found", status: 404 }), {
        status: 404,
        headers: { "Content-Type": "application/problem+json" },
      });
  }
}

function check(ok, what) {
  if (!ok) throw new Error(`${runtime}: ${what}`);
}

const dir = await mkdtemp(join(tmpdir(), "mybot-runtimes-"));
try {
  const path = join(dir, "state.json");
  const bot = new Bot({
    server: "https://example.com",
    fetch: fakeFetch,
    tokenSource: staticToken("token"),
    store: new FileStore(path),
    logger: { debug() {}, info() {}, warn() {}, error: console.error },
    mode: "polling",
    pollInterval: 10,
  });
  bot.onCommand("ping", "answer pong", async (ctx) => {
    await ctx.reply("pong");
  });
  const controller = new AbortController();
  const run = bot.run({ signal: controller.signal });
  const deadline = Date.now() + 10_000;
  while (!(await bot.store.isProcessed(notificationId))) {
    check(Date.now() < deadline, "the mention was not processed");
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  controller.abort();
  await run;

  check(posts.length === 1, `posted ${String(posts.length)} replies, want 1`);
  check(posts[0].key === `mybot:reply:${notificationId}:1`, `the Idempotency-Key is ${String(posts[0].key)}`);
  check(posts[0].body.text === "@alice pong" && posts[0].body.visibility === "unlisted", `the reply is ${JSON.stringify(posts[0].body)}`);
  const state = JSON.parse(await readFile(path, "utf8"));
  check(typeof state.processed[notificationId] === "string" && state.position === notificationId, `the state file is ${JSON.stringify(state)}`);
  check(plainText("<p>a &amp; b</p>") === "a & b", "plainText");
  console.log(`${runtime}: @mycroblog/mybot loads and runs a bot`);
} finally {
  await rm(dir, { recursive: true, force: true });
}
