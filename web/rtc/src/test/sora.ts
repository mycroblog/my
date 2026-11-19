// SPDX-License-Identifier: AGPL-3.0-only
// A fake of the parts of sora-js-sdk that the Sora session uses: connections that record their URLs, role, channel,
// metadata and options, and how they were joined, left and given a new track; whose events the test emits;
// `loadFakeSora` stands in for the SDK's import.
import { mock } from "bun:test";
import type { SoraSdk } from "../types";
import { statsReport } from "./fake";

type Listener = (event: unknown) => void;

export class FakeSoraConnection {
  static all: FakeSoraConnection[] = [];
  static failNext = false;
  readonly listeners = new Map<string, Listener>();
  readonly connect = mock((_stream?: unknown) =>
    FakeSoraConnection.failNext ? Promise.reject(new Error("refused")) : Promise.resolve(),
  );
  readonly disconnect = mock(() => {
    // The SDK calls the disconnect callback for a disconnect of its own too.
    this.listeners.get("disconnect")?.({ type: "normal", title: "DISCONNECT" });
    return Promise.resolve();
  });
  readonly replaceAudioTrack = mock((_stream: unknown, _track: unknown) => Promise.resolve());
  readonly pc = { getStats: () => Promise.resolve(this.stats) };
  stats: RTCStatsReport = statsReport([]);
  readonly urls: string[];
  readonly role: string;
  readonly channel: string;
  readonly metadata: unknown;
  readonly options: unknown;

  constructor(urls: string[], role: string, channel: string, metadata: unknown, options: unknown) {
    this.urls = urls;
    this.role = role;
    this.channel = channel;
    this.metadata = metadata;
    this.options = options;
    FakeSoraConnection.all.push(this);
  }

  on(kind: string, listener: Listener): void {
    this.listeners.set(kind, listener);
  }

  emit(kind: string, event: unknown): void {
    this.listeners.get(kind)?.(event);
  }

  /** A remote connection's audio arrives in the stream of its connection ID. */
  track(connectionId: string): void {
    this.emit("track", { track: { kind: "audio" }, streams: [{ id: connectionId }] });
  }

  /** Sora names the client_id of connections. */
  created(connections: { connection_id: string; client_id: string }[]): void {
    this.emit("notify", { type: "notify", event_type: "connection.created", data: connections });
  }
}

const sdk = {
  connection: (urls: string[]) => ({
    sendrecv: (channel: string, metadata: unknown, options: unknown) =>
      new FakeSoraConnection(urls, "sendrecv", channel, metadata, options),
    recvonly: (channel: string, metadata: unknown, options: unknown) =>
      new FakeSoraConnection(urls, "recvonly", channel, metadata, options),
  }),
};

/** Loads the fake SDK, as the session's `import("sora-js-sdk")` would. */
export function loadFakeSora(): Promise<SoraSdk> {
  return Promise.resolve(sdk as unknown as SoraSdk);
}

/** The connection the session made `index`-th. */
export function sora(index: number): FakeSoraConnection {
  const found = FakeSoraConnection.all[index];
  if (found === undefined) throw new Error(`no connection ${String(index)}`);
  return found;
}

export function resetFakeSora(): void {
  FakeSoraConnection.all = [];
  FakeSoraConnection.failNext = false;
}
