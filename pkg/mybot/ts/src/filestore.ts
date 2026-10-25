// SPDX-License-Identifier: MIT
// FileStore, the one part of the library that touches the file system. It loads node:fs/promises on first use, with
// a dynamic import, which Node.js, Bun, and Deno all provide (Deno needs --allow-read and --allow-write for the file),
// so that the rest of the library runs wherever fetch does.
import { decodeState, emptyState, encodeState, StateStore, type StoreState } from "./store.js";

/** The file of the FileStore that a Bot uses when its options name no store. */
export const defaultStatePath = "./mybot-state.json";

/** The part of node:fs/promises that FileStore uses. */
type FileHandle = {
  writeFile(data: string, encoding: "utf8"): Promise<void>;
  sync(): Promise<void>;
  close(): Promise<void>;
};
type Fs = {
  readFile(path: string, encoding: "utf8"): Promise<string>;
  open(path: string, flags: string, mode: number): Promise<FileHandle>;
  rename(from: string, to: string): Promise<void>;
  rm(path: string, options: { force: boolean }): Promise<void>;
};

/** The module name, in a variable so that the type checker, which has no Node.js types here, does not resolve it. */
const fsModule = "node:fs/promises";

let fsPromise: Promise<Fs> | null = null;

/** node:fs/promises, loaded once. */
function fs(): Promise<Fs> {
  fsPromise ??= import(fsModule) as Promise<Fs>;
  return fsPromise;
}

/** Whether error is the error of a missing file. */
function notFound(error: unknown): boolean {
  return typeof error === "object" && error !== null && (error as { code?: unknown }).code === "ENOENT";
}

/** The directory part of path ("." for a bare name), for either separator. */
function dirname(path: string): string {
  const i = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  if (i < 0) return ".";
  return i === 0 ? path.slice(0, 1) : path.slice(0, i);
}

/** The last part of path. */
function basename(path: string): string {
  return path.slice(Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\")) + 1);
}

/**
 * A Store in one JSON file, the default Store of a Bot. It reads the file on first use (a missing file is an empty
 * state, created on the first change). Every change rewrites the file: the new content goes to a temporary file in
 * the same directory, which then replaces the file, so that a crash never leaves half of it. The file holds the
 * refresh token: it is created readable by its owner only. The document is that of the Go library's FileStore.
 */
export class FileStore extends StateStore {
  /** The file of the store. */
  readonly path: string;
  #state: Promise<StoreState> | null = null;
  /** The writes in order: each rewrites the whole file, so the last one wins. */
  #writes: Promise<void> = Promise.resolve();

  constructor(path: string = defaultStatePath) {
    super();
    this.path = path;
  }

  protected load(): Promise<StoreState> {
    this.#state ??= this.#read();
    return this.#state;
  }

  async #read(): Promise<StoreState> {
    const f = await fs();
    let text: string;
    try {
      text = await f.readFile(this.path, "utf8");
    } catch (error) {
      if (notFound(error)) return emptyState();
      throw new Error(`mybot: read the state file ${this.path}`, { cause: error });
    }
    try {
      return decodeState(text);
    } catch (error) {
      throw new Error(`mybot: read the state file ${this.path}`, { cause: error });
    }
  }

  protected save(state: StoreState): Promise<void> {
    const text = encodeState(state);
    const write = this.#writes.then(() => this.#write(text));
    // A failed write fails its caller, not the next write.
    this.#writes = write.catch(() => undefined);
    return write;
  }

  async #write(text: string): Promise<void> {
    const f = await fs();
    const tmp = `${dirname(this.path)}/.${basename(this.path)}.${crypto.randomUUID()}.tmp`;
    try {
      const handle = await f.open(tmp, "wx", 0o600);
      try {
        await handle.writeFile(text, "utf8");
        await handle.sync();
      } finally {
        await handle.close();
      }
      await f.rename(tmp, this.path);
    } catch (error) {
      await f.rm(tmp, { force: true }).catch(() => undefined);
      throw new Error(`mybot: write the state file ${this.path}`, { cause: error });
    }
  }
}
