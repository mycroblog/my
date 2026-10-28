// SPDX-License-Identifier: AGPL-3.0-only
// The configuration of the `my serve` that e2e/server.ts starts, and `bin/my` commands against its
// database: e2e/server.ts seeds the database with them, and the specs of the main flow
// (e2e/main-flow.spec.ts) create users and approve sign-ups with them while the server runs, as an
// operator does (docs/operations/registration.md). The voice chat server (`node e2e/server.ts voice`) has the same
// configuration with `voice = p2p`, on a database and a port of its own. The demo server (`node e2e/server.ts demo`,
// ADR-0117 §3) is the test build with the fake payment provider, in the `development` profile, with `voice = p2p`
// and its email sent to the development Mailpit on 127.0.0.1 (scripts/task.sh docker:mailpit).
import { execFileSync } from "node:child_process";
import { join, resolve } from "node:path";
import {
  demoServerDir,
  demoServerPort,
  serverDir,
  serverKEK,
  serverPort,
  voiceServerDir,
  voiceServerPort,
} from "./server-origin.ts";

const root = resolve(import.meta.dirname, "../../..");

/** The binary `scripts/task.sh go:bin` writes; E2E_MY_BIN overrides it. */
export const myBin = process.env["E2E_MY_BIN"] ?? join(root, "bin/my");

/**
 * Which server of e2e/server.ts: the one every spec but e2e/voice-p2p.spec.ts uses, the one with voice chat on, or the
 * demo server with the demo scenario.
 */
export type E2EServer = "main" | "voice" | "demo";

/** The directory of the database and the blobs of `server`. */
export function serverDirOf(server: E2EServer): string {
  return { main: serverDir, voice: voiceServerDir, demo: demoServerDir }[server];
}

/** The ports of the development Mailpit (deploy/docker/mailpit), which MAILPIT_SMTP_PORT and MAILPIT_UI_PORT move. */
export const mailpit = {
  smtpPort: Number(process.env["MAILPIT_SMTP_PORT"] ?? "1025"),
  uiPort: Number(process.env["MAILPIT_UI_PORT"] ?? "8025"),
  /** The development CA of scripts/mailpit-cert.sh, which `my serve` trusts for STARTTLS. */
  ca: join(process.env["MAILPIT_CERTS"] ?? join(root, ".tmp/mailpit"), "ca.pem"),
} as const;

/** The file of the demo server's key-encryption key (MY_KEK_FILE), which e2e/server.ts writes. */
export const demoKEKFile = join(demoServerDir, "kek");

/** The SQLite database of `server`. */
export function serverDatabaseOf(server: E2EServer): string {
  return join(serverDirOf(server), "my.db");
}

/** The environment of `my serve` and of every `bin/my` command against the database of `server`. */
export function serverEnv(server: E2EServer = "main"): NodeJS.ProcessEnv {
  const port = String({ main: serverPort, voice: voiceServerPort, demo: demoServerPort }[server]);
  const host = `localhost:${port}`;
  return {
    ...process.env,
    MY_HOST: host,
    MY_LISTEN: `127.0.0.1:${port}`,
    MY_WEB_ORIGIN: `http://${host}`,
    MY_DB: "sqlite",
    MY_DATABASE_URL: serverDatabaseOf(server),
    MY_BLOB_FS_DIR: join(serverDirOf(server), "blobs"),
    // The demo server reads its KEK from a file that e2e/server.ts writes: Bun, which runs the demo seed and its
    // `bin/my` commands, drops environment values that end in "=", as base64 keys do.
    ...(server === "demo" ? { MY_KEK_FILE: demoKEKFile } : { MY_KEK: serverKEK }),
    MY_MAIL: "none",
    // Voice chat between browsers on one machine: P2P without STUN or TURN, as the host candidates suffice.
    ...(server !== "main" && { MY_VOICE: "p2p" }),
    ...(server === "demo" && {
      MY_PROFILE: "development",
      MY_PAYMENTS_PROVIDER: "fake",
      MY_MAIL: "smtp",
      MY_MAIL_FROM: "noreply@demo.test",
      MY_SMTP_HOST: "localhost",
      MY_SMTP_PORT: String(mailpit.smtpPort),
      // mail = smtp verifies Mailpit's certificate: trust the development CA instead of the system roots, which also
      // keeps the server off outside HTTPS servers.
      SSL_CERT_FILE: mailpit.ca,
    }),
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
