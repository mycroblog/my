// SPDX-License-Identifier: AGPL-3.0-only
// Starts the real server for the E2E of the server-rendered pages (ADR-0030) and of the main flow: a fresh
// SQLite database in the run's temporary directory (e2e/server-origin.ts) with schema/sqlite applied,
// the admin, the documents, and the third-party OAuth client of e2e/server-origin.ts, then
// `my serve -pseudo-locales` (so that the en-XA and ar-XB projects get their locale) until Playwright stops it.
// With the argument `voice` (`node e2e/server.ts voice`) it starts the server of e2e/voice-p2p.spec.ts instead:
// `voice = p2p` on a database and a port of its own, where every user may start a voice chat room
// (`voice_room_starters = everyone`), so that the other specs keep a server with voice chat off.
// Playwright runs both as webServers with Node.js, from the binary `scripts/task.sh go:bin` writes (override with
// E2E_MY_BIN), in the environment of e2e/server-cli.ts.
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { mkdirSync, readdirSync, readFileSync, rmSync } from "node:fs";
import { join, resolve } from "node:path";
import { DatabaseSync } from "node:sqlite";
import { type E2EServer, myBin, runMy, serverDatabaseOf, serverDirOf, serverEnv } from "./server-cli.ts";
import { admin, documents, thirdPartyClient } from "./server-origin.ts";

const which: E2EServer = process.argv[2] === "voice" ? "voice" : "main";
const root = resolve(import.meta.dirname, "../../..");
const database = serverDatabaseOf(which);
const env = serverEnv(which);
mkdirSync(env["MY_BLOB_FS_DIR"] ?? "", { recursive: true });

// The schema files are plain SQL, executed in name order as Atlas does in production (ADR-0023, ADR-0082).
const schemaDir = join(root, "schema/sqlite");
const db = new DatabaseSync(database);
for (const name of readdirSync(schemaDir).filter((n) => n.endsWith(".sql")).toSorted()) {
  db.exec(readFileSync(join(schemaDir, name), "utf8"));
}
db.close();

if (which === "voice") {
  // The spec creates its users with `bin/my`; any of them may open a room.
  runMy(["instance", "set", "voice_room_starters", "everyone"], "", which);
} else {
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

const server = spawn(myBin, ["serve", "-pseudo-locales"], { env, stdio: "inherit" });
const stop = () => {
  server.kill("SIGTERM");
};
process.on("SIGINT", stop);
process.on("SIGTERM", stop);
server.on("exit", (code, signal) => {
  rmSync(serverDirOf(which), { recursive: true, force: true });
  process.exit(signal === null ? (code ?? 1) : 0);
});
