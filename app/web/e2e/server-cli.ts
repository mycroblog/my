// SPDX-License-Identifier: AGPL-3.0-only
// The configuration of the `my serve` that e2e/server.ts starts, and `bin/my` commands against its
// database: e2e/server.ts seeds the database with them, and the specs of the main flow
// (e2e/main-flow.spec.ts) create users and approve sign-ups with them while the server runs, as an
// operator does (docs/operations/registration.md). The voice chat server (`node e2e/server.ts voice`) has the same
// configuration with `voice = p2p`, on a database and a port of its own.
import { execFileSync } from "node:child_process";
import { join, resolve } from "node:path";
import { serverDir, serverKEK, serverPort, voiceServerDir, voiceServerPort } from "./server-origin.ts";

const root = resolve(import.meta.dirname, "../../..");

/** The binary `scripts/task.sh go:bin` writes; E2E_MY_BIN overrides it. */
export const myBin = process.env["E2E_MY_BIN"] ?? join(root, "bin/my");

/** Which server of e2e/server.ts: the one every spec but e2e/voice-p2p.spec.ts uses, or the one with voice chat on. */
export type E2EServer = "main" | "voice";

/** The directory of the database and the blobs of `server`. */
export function serverDirOf(server: E2EServer): string {
  return server === "voice" ? voiceServerDir : serverDir;
}

/** The SQLite database of `server`. */
export function serverDatabaseOf(server: E2EServer): string {
  return join(serverDirOf(server), "my.db");
}

/** The environment of `my serve` and of every `bin/my` command against the database of `server`. */
export function serverEnv(server: E2EServer = "main"): NodeJS.ProcessEnv {
  const port = String(server === "voice" ? voiceServerPort : serverPort);
  const host = `localhost:${port}`;
  return {
    ...process.env,
    MY_HOST: host,
    MY_LISTEN: `127.0.0.1:${port}`,
    MY_WEB_ORIGIN: `http://${host}`,
    MY_DB: "sqlite",
    MY_DATABASE_URL: serverDatabaseOf(server),
    MY_BLOB_FS_DIR: join(serverDirOf(server), "blobs"),
    MY_KEK: serverKEK,
    MY_MAIL: "none",
    // Voice chat between browsers on one machine: P2P without STUN or TURN, as the host candidates suffice.
    ...(server === "voice" && { MY_VOICE: "p2p" }),
  };
}

/**
 * Runs `bin/my` with `args` against the database of `server`, with `input` on its standard input, and
 * returns its standard output. A failing command throws with its standard error.
 */
export function runMy(args: readonly string[], input = "", server: E2EServer = "main"): string {
  return execFileSync(myBin, args, {
    env: serverEnv(server),
    input,
    encoding: "utf8",
    stdio: ["pipe", "pipe", "pipe"],
  });
}

/** Whether the specs reach the server e2e/server.ts starts, whose database `runMy` can change. */
export const ownsServer = process.env["E2E_SERVER_URL"] === undefined;
