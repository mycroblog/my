// SPDX-License-Identifier: AGPL-3.0-only

// The batches of the dev server: one JSON file per batch in .tmp/feedback, which `scripts/agentkit.sh feedback`
// reads and answers in place, and the URL of the dev server beside them.

import { existsSync, mkdirSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import type { Batch, BatchInput } from "./note";

/** The directory of the batches of the workspace at `root`. */
export const feedbackDir = (root: string) => join(root, ".tmp", "feedback");

const pad = (n: number) => String(n).padStart(2, "0");

/** A batch id that sorts by time: 20261112-153045-a1b2. */
export function batchId(now: Date, random: () => number = Math.random): string {
  const date = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}`;
  const time = `${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  const suffix = Math.floor(random() * 0x10000)
    .toString(16)
    .padStart(4, "0");
  return `${date}-${time}-${suffix}`;
}

/** The name of the package that holds `file` (a path relative to `root`): the layer of the component. */
export function layerOf(root: string, file: string, cache = new Map<string, string | null>()): string | null {
  let dir = dirname(join(root, file));
  const seen: string[] = [];
  let layer: string | null = null;
  while (dir.startsWith(root) && dir !== root) {
    const cached = cache.get(dir);
    if (cached !== undefined) {
      layer = cached;
      break;
    }
    seen.push(dir);
    const manifest = join(dir, "package.json");
    if (existsSync(manifest)) {
      const { name } = JSON.parse(readFileSync(manifest, "utf8")) as { name?: string };
      layer = name ?? null;
      break;
    }
    dir = dirname(dir);
  }
  for (const d of seen) cache.set(d, layer);
  return layer;
}

const fileOf = (source: string) => source.replace(/:\d+$/, "");

/** Writes a batch from what the overlay sent and returns it. */
export function writeBatch(root: string, input: BatchInput, now = new Date()): Batch {
  const dir = feedbackDir(root);
  mkdirSync(dir, { recursive: true });
  const id = batchId(now);
  const cache = new Map<string, string | null>();
  const batch: Batch = {
    id,
    sentAt: now.toISOString(),
    page: input.page,
    notes: input.notes.map((note, i) => ({
      id: `${id}.${i + 1}`,
      ...note,
      layer: note.source === null ? null : layerOf(root, fileOf(note.source), cache),
    })),
  };
  const path = join(dir, `${id}.json`);
  writeFileSync(`${path}.tmp`, `${JSON.stringify(batch, null, 2)}\n`);
  renameSync(`${path}.tmp`, path);
  return batch;
}

/** The file of the feedback directory in which the dev server records its URL while it listens; not a batch. */
const serverFile = "server.json";

/**
 * Records the URL of the dev server, which `scripts/agentkit.sh feedback url` prints, and the pid of its process,
 * which `scripts/agentkit.sh tidy` stops before it removes the worktree.
 */
export function writeServer(root: string, url: string, pid: number): void {
  const dir = feedbackDir(root);
  mkdirSync(dir, { recursive: true });
  const path = join(dir, serverFile);
  writeFileSync(`${path}.tmp`, `${JSON.stringify({ url, pid })}\n`);
  renameSync(`${path}.tmp`, path);
}

/** Removes the record of the dev server. */
export function removeServer(root: string): void {
  rmSync(join(feedbackDir(root), serverFile), { force: true });
}

/** Every batch, oldest first. */
export function readBatches(root: string): Batch[] {
  const dir = feedbackDir(root);
  if (!existsSync(dir)) return [];
  return readdirSync(dir)
    .filter((f) => f.endsWith(".json") && f !== serverFile)
    .sort()
    .flatMap((f) => {
      try {
        return [JSON.parse(readFileSync(join(dir, f), "utf8")) as Batch];
      } catch {
        // A file being replaced, or one that is not a batch: the next read sees it whole.
        return [];
      }
    });
}
