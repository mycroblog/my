// SPDX-License-Identifier: AGPL-3.0-only

// The demo server (ADR-0015 §7, §8), which the decider's demo (`scripts/task.sh web:demo`, main.ts) and the E2E tests
// (specs/global-setup.ts) both start: the test build of `bin/my` (GO_TAGS=e2e_fakes, with the web client of web:build
// embedded) on a fresh SQLite database and blob directory, with the schema applied by `my schema apply`, in the
// `development` profile with the settings that need no outside service (no mail, the fake payments, voice chat between
// browsers on one machine, federation off), seeded with the scenario (seed/main.ts). The directory is made anew on
// every start: the fake payments keep their side in the server's memory alone.

import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { existsSync, mkdirSync, openSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { SeedState } from "../seed/seed";
import { myBin, root } from "./paths";

export type DemoOptions = {
  /** The directory of the database, the blobs, the key, the logs and the seed's state; emptied first. */
  dir: string;
  /** The port; a free one by default. */
  port?: number;
  /** Where the server and the seed write: this process's output, or the file server.log in `dir`. */
  output: "inherit" | "file";
};

export type Demo = {
  /** The server's origin, on localhost, whose Secure cookies a browser takes over plain HTTP. */
  url: string;
  /** The seed's state: what the server decided that the tests need (seed/seed.ts). */
  state: SeedState;
  /** The file that holds `state`. */
  stateFile: string;
  /** Stops the server and removes the directory. */
  stop: () => Promise<void>;
};

/** `port` when nothing listens on it now, else an error; a port the system picks when `port` is 0. */
function freePort(port: number): number {
  let probe;
  try {
    probe = Bun.serve({ port, hostname: "127.0.0.1", fetch: () => new Response() });
  } catch {
    throw new Error(`demo: something already listens on port ${port}`);
  }
  const picked = probe.port;
  void probe.stop(true);
  if (picked === undefined) throw new Error("demo: no free port");
  return picked;
}

/** The environment of `my serve` and of every `bin/my` command against its database. */
function serverEnv(dir: string, port: number): Record<string, string> {
  const host = `localhost:${port}`;
  // The settings come from here alone, not from a server's settings this process's environment may hold.
  const inherited = Object.entries(process.env).filter(
    (e): e is [string, string] => !e[0].startsWith("MY_") && e[1] !== undefined,
  );
  return {
    ...Object.fromEntries(inherited),
    MY_PROFILE: "development",
    MY_HOST: host,
    MY_LISTEN: `127.0.0.1:${port}`,
    MY_WEB_ORIGIN: `http://${host}`,
    MY_DATABASE: "sqlite",
    MY_DATABASE_URL: join(dir, "my.db"),
    MY_BLOB: "fs",
    MY_BLOB_FS_DIR: join(dir, "blobs"),
    // A file: Bun drops an environment value that ends in "=", as a base64 key may.
    MY_KEK_FILE: join(dir, "kek"),
    MY_MAIL: "none",
    MY_PAYMENTS: "fake",
    MY_VOICE: "p2p",
  };
}

async function ready(url: string, server: ChildProcess): Promise<void> {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    if (server.exitCode !== null) throw new Error(`demo: my serve exited with ${server.exitCode}`);
    try {
      if ((await fetch(`${url}/readyz`)).ok) return;
    } catch {
      // Not listening yet.
    }
    await new Promise((done) => setTimeout(done, 200));
  }
  throw new Error("demo: my serve did not answer /readyz within a minute");
}

/** Starts the demo server, seeds it, and returns once the scenario is in. */
export async function startDemo(o: DemoOptions): Promise<Demo> {
  if (!existsSync(myBin))
    throw new Error(`demo: ${myBin} is missing; build it with scripts/task.sh go:bin GO_TAGS=e2e_fakes`);
  rmSync(o.dir, { recursive: true, force: true });
  mkdirSync(join(o.dir, "blobs"), { recursive: true });
  const port = freePort(o.port ?? 0);
  const url = `http://localhost:${port}`;
  const env = serverEnv(o.dir, port);
  writeFileSync(env["MY_KEK_FILE"] ?? "", randomBytes(32).toString("base64"), { mode: 0o600 });
  const log = join(o.dir, "server.log");
  const stdio = o.output === "inherit" ? "inherit" : openSync(log, "a");
  /** The error `err`, after the log of the server and the seed when it went to a file, which stop() removes. */
  const failed = (err: unknown) => {
    if (o.output === "file" && existsSync(log)) process.stderr.write(readFileSync(log, "utf8"));
    return err;
  };
  try {
    // The statements of the schema, hundreds of lines, are left out of the output.
    execFileSync(myBin, ["schema", "apply"], { env, stdio: ["ignore", "ignore", stdio] });
    execFileSync(myBin, ["db", "upgrade"], { env, stdio: ["ignore", stdio, stdio] });
  } catch (err) {
    throw failed(err);
  }

  const server = spawn(myBin, ["serve"], { env, stdio: ["ignore", stdio, stdio] });
  const exited = new Promise<void>((done) =>
    server.once("exit", () => {
      done();
    }),
  );
  const stop = async () => {
    if (server.exitCode === null && server.signalCode === null) server.kill("SIGTERM");
    await exited;
    rmSync(o.dir, { recursive: true, force: true });
  };
  try {
    await ready(url, server);
    const stateFile = join(o.dir, "seed-state.json");
    const seeder = spawn(
      process.execPath,
      [
        "--conditions=@mycroblog/source",
        join(import.meta.dirname, "../seed/main.ts"),
        "--server",
        url,
        "--state",
        stateFile,
      ],
      { env: { ...env, E2E_MY_BIN: myBin }, stdio: ["ignore", stdio, stdio], cwd: root },
    );
    const code = await new Promise<number | null>((done) => seeder.once("exit", done));
    if (code !== 0) throw new Error(`demo: the seed failed (exit ${code})`);
    const state = JSON.parse(readFileSync(stateFile, "utf8")) as SeedState;
    return { url, state, stateFile, stop };
  } catch (err) {
    failed(err);
    await stop();
    throw err;
  }
}
