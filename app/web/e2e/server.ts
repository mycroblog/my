// SPDX-License-Identifier: AGPL-3.0-only
// Starts the real server for the E2E of the server-rendered pages (ADR-0030) and of the main flow: a fresh
// SQLite database in the run's temporary directory (e2e/server-origin.ts) with schema/sqlite applied,
// the admin, the documents, and the third-party OAuth client of e2e/server-origin.ts, then
// `my serve -pseudo-locales` (so that the en-XA and ar-XB projects get their locale) until Playwright stops it.
// With the argument `voice` (`node e2e/server.ts voice`) it starts the server of e2e/voice-p2p.spec.ts instead:
// `voice = p2p` on a database and a port of its own, where every user may start a voice chat room
// (`voice_room_starters = everyone`), so that the other specs keep a server with voice chat off.
// With the argument `demo` it starts the demo server (ADR-0117 §3, §4): the test build (`GO_TAGS=e2e_fakes`) with the
// fake payment provider and its email sent to the development Mailpit, on a fresh database and a port of its own.
// Once the server answers, demo/seed.ts (run by Bun, which reads @mycroblog/mykit from its sources) applies the demo
// scenario, and the script prints "demo: ready", which Playwright and `scripts/task.sh web:demo` wait for. It needs
// Mailpit (`scripts/task.sh docker:mailpit`) and stops at once without it. The demo server has Web Push on (ADR-0118
// §3): a VAPID key generated on every start (`my config vapid-key`), and the fake push service of the test build
// (`my dev push-service`, with the certificate that docker:mailpit's scripts/mailpit-cert.sh writes), which starts
// before the server and stops with it; push_allowed_destinations lets the server's Web Push sends reach it alone.
// Playwright runs them as webServers with Node.js, from the binary `scripts/task.sh go:bin` writes (override with
// E2E_MY_BIN), in the environment of e2e/server-cli.ts.
import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { connect } from "node:net";
import { join, resolve } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { personas } from "../demo/scenario.ts";
import {
  demoKEKFile,
  demoVAPIDKeyFile,
  type E2EServer,
  mailpit,
  myBin,
  pushService,
  runMy,
  serverDatabaseOf,
  serverDirOf,
  serverEnv,
} from "./server-cli.ts";
import { admin, demoServerURL, documents, serverKEK, thirdPartyClient } from "./server-origin.ts";

const argument = process.argv[2];
const which: E2EServer = argument === "voice" || argument === "demo" ? argument : "main";
const root = resolve(import.meta.dirname, "../../..");
const database = serverDatabaseOf(which);
const env = serverEnv(which);

/** Whether something accepts TCP connections on 127.0.0.1:port. */
function listening(port: number): Promise<boolean> {
  return new Promise((done) => {
    const socket = connect({ host: "127.0.0.1", port });
    socket.setTimeout(2000);
    socket.once("connect", () => {
      socket.destroy();
      done(true);
    });
    socket.once("error", () => {
      done(false);
    });
    socket.once("timeout", () => {
      socket.destroy();
      done(false);
    });
  });
}

if (which === "demo") {
  if (!existsSync(mailpit.ca) || !(await listening(mailpit.smtpPort)) || !(await listening(mailpit.uiPort))) {
    console.error(
      `e2e/server.ts demo: the demo server sends its email to Mailpit, but nothing listens on 127.0.0.1:${String(mailpit.smtpPort)} (SMTP) and :${String(mailpit.uiPort)} (web UI), or ${mailpit.ca} is missing; start it with scripts/task.sh docker:mailpit`,
    );
    process.exit(1);
  }
  if (!existsSync(pushService.cert) || !existsSync(pushService.key)) {
    console.error(
      `e2e/server.ts demo: the fake push service's certificate ${pushService.cert} is missing; create it with scripts/mailpit-cert.sh (scripts/task.sh docker:mailpit runs it)`,
    );
    process.exit(1);
  }
  if (await listening(pushService.port)) {
    console.error(
      `e2e/server.ts demo: something already listens on 127.0.0.1:${String(pushService.port)}, the port of the fake push service (E2E_PUSH_SERVICE_PORT moves it)`,
    );
    process.exit(1);
  }
  // A fresh database on every start: the fake provider keeps its side in the server's memory only (ADR-0117 §4).
  rmSync(serverDirOf(which), { recursive: true, force: true });
}
mkdirSync(env["MY_BLOB_FS_DIR"] ?? "", { recursive: true });
if (which === "demo") {
  writeFileSync(demoKEKFile, serverKEK, { mode: 0o600 });
  // `my config vapid-key` prints the line `vapid_private_key = <key>`.
  const vapid = /^vapid_private_key\s*=\s*(\S+)$/m.exec(
    execFileSync(myBin, ["config", "vapid-key"], { encoding: "utf8" }),
  )?.[1];
  if (vapid === undefined) {
    console.error("e2e/server.ts demo: `my config vapid-key` printed no vapid_private_key");
    process.exit(1);
  }
  writeFileSync(demoVAPIDKeyFile, vapid, { mode: 0o600 });
}

// The schema files are plain SQL, executed in name order as Atlas does in production (ADR-0023, ADR-0082).
const schemaDir = join(root, "schema/sqlite");
const db = new DatabaseSync(database);
for (const name of readdirSync(schemaDir)
  .filter((n) => n.endsWith(".sql"))
  .toSorted()) {
  db.exec(readFileSync(join(schemaDir, name), "utf8"));
}
db.close();

if (which === "voice") {
  // The spec creates its users with `bin/my`; any of them may open a room.
  runMy(["instance", "set", "voice_room_starters", "everyone"], "", which);
} else if (which === "main") {
  runMy(["admin", "create", "-username", "alice", "-email", admin.email], `${admin.password}\n`);

  // The documents, one text each that every locale shows, so that every project's pages link to them (#489).
  runMy(["instance", "set", "terms", "-"], documents.terms);
  runMy(["instance", "set", "privacy_policy", "-"], documents.privacyPolicy);

  // Third-party clients have no registration flow yet, so the client is written to its tables directly
  // (data-model/04-tables/01-identity.md §4.1): a public client limited to the read scope with one redirect URI.
  const seed = new DatabaseSync(database);
  const id = randomUUID();
  seed
    .prepare(
      "INSERT INTO oauth_clients (id, client_id, client_secret_hash, name, website, scope, registration, owner_user_id, created_at) VALUES (?, ?, NULL, ?, NULL, 'read', 'dynamic', NULL, ?)",
    )
    .run(id, thirdPartyClient.clientID, thirdPartyClient.name, Date.now());
  seed
    .prepare("INSERT INTO oauth_client_redirect_uris (client_id, redirect_uri) VALUES (?, ?)")
    .run(id, thirdPartyClient.redirectURI);
  seed.close();
}

// The demo server's fake push service, which accepts the server's Web Push sends; it runs as long as the server does.
let pushServiceProcess: ChildProcess | undefined;
if (which === "demo") {
  const fake = spawn(
    myBin,
    [
      "dev",
      "push-service",
      "-listen",
      `127.0.0.1:${String(pushService.port)}`,
      "-origin",
      pushService.origin,
      "-cert",
      pushService.cert,
      "-key",
      pushService.key,
    ],
    { env, stdio: ["ignore", "pipe", "inherit"] },
  );
  pushServiceProcess = fake;
  // It prints "my dev push-service: listening on ..." once it accepts connections (a TCP probe would log a failed
  // TLS handshake), and its output goes on to ours.
  const up = await new Promise<boolean>((done) => {
    const timer = setTimeout(() => {
      done(false);
    }, 10_000);
    fake.stdout.on("data", (chunk: Buffer) => {
      process.stdout.write(chunk);
      if (chunk.toString("utf8").includes("listening on")) {
        clearTimeout(timer);
        done(true);
      }
    });
    fake.once("exit", () => {
      clearTimeout(timer);
      done(false);
    });
  });
  if (!up) {
    fake.kill("SIGTERM");
    console.error(
      "e2e/server.ts demo: my dev push-service did not start (is bin/my the test build, GO_TAGS=e2e_fakes?)",
    );
    rmSync(serverDirOf(which), { recursive: true, force: true });
    process.exit(1);
  }
}

// The demo server keeps the pseudo-locales off: it is what people see.
const server = spawn(myBin, which === "demo" ? ["serve"] : ["serve", "-pseudo-locales"], { env, stdio: "inherit" });
const stop = () => {
  server.kill("SIGTERM");
};
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
// The fake push service stopping on its own leaves the server without its push service: stop the server too.
// A Ctrl-C or a signal to the process group stops both, the fake with status 0.
pushServiceProcess?.on("exit", (code) => {
  if (server.exitCode === null && server.signalCode === null) {
    if (code !== 0) console.error("e2e/server.ts demo: my dev push-service failed; stopping the demo server");
    stop();
  }
});
server.on("exit", (code, signal) => {
  pushServiceProcess?.kill("SIGTERM");
  rmSync(serverDirOf(which), { recursive: true, force: true });
  process.exit(signal === null ? (code ?? 1) : 0);
});

/** Waits until the demo server answers /readyz, for at most a minute. */
async function ready(): Promise<boolean> {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    try {
      if ((await fetch(`${demoServerURL}/readyz`)).ok) return true;
    } catch {
      // Not listening yet.
    }
    await new Promise((done) => setTimeout(done, 200));
  }
  return false;
}

if (which === "demo") {
  if (!(await ready())) {
    console.error("e2e/server.ts demo: my serve did not answer /readyz within a minute");
    stop();
  } else {
    const seeder = spawn(
      "bun",
      ["--conditions=@mycroblog/source", join(import.meta.dirname, "../demo/seed.ts"), "--server", demoServerURL],
      { env: { ...env, DEMO_MY_BIN: myBin }, stdio: "inherit" },
    );
    seeder.on("exit", (code) => {
      if (code === 0) {
        const logins = personas.map((p) => `  ${p.username.padEnd(12)} ${p.email.padEnd(24)} ${p.password}`);
        console.log(
          [
            `demo: ready at ${demoServerURL}`,
            `Mailpit (the server's email): http://127.0.0.1:${String(mailpit.uiPort)}`,
            `Web Push: on, sent to the fake push service at ${pushService.origin}`,
            "Sign in as (email, password):",
            ...logins,
          ].join("\n"),
        );
      } else {
        console.error("e2e/server.ts demo: demo/seed.ts failed to apply the demo scenario");
        stop();
      }
    });
  }
}
