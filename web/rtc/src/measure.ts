// SPDX-License-Identifier: AGPL-3.0-only

// The quality of the calls as UX measurements (ADR-0167 §1, §2): every RoomSession that `createRoomSession` makes
// reports the round-trip time, the jitter and the packet loss of its media path, named by what carries the audio
// (`p2p`, `sfu`, `livekit`, `sora`), to where the app says (`measureCallsWith`). It reports the first quality the
// session hears and then one in `qualityReportEvery` (about one a minute at the default interval), so that a long call
// does not fill the reports. The caller of a session writes no code for it; without a destination nothing is reported.

import type { ConnectionQuality, RoomKind, RoomSession } from "./types";

/** One sample of a call's quality: milliseconds, or the percentage of the packets lost (0 to 100). */
export type CallSample = {
  metric: "call_rtt" | "call_jitter" | "call_packet_loss";
  name: RoomKind;
  value: number;
};

/** How many of a session's quality samples make one report: the first, then one in this many. */
export const qualityReportEvery = 12;

let sink: ((sample: CallSample) => void) | null = null;

/** Gives the quality of every call to `report` until the function it returns is called. */
export function measureCallsWith(report: (sample: CallSample) => void): () => void {
  sink = report;
  return () => {
    if (sink === report) sink = null;
  };
}

/** The samples of `quality`, leaving out what the statistics did not say. */
export function callSamples(kind: RoomKind, { rtt, jitter, packetLoss }: ConnectionQuality): CallSample[] {
  const samples: CallSample[] = [];
  if (rtt !== null) samples.push({ metric: "call_rtt", name: kind, value: rtt });
  if (jitter !== null) samples.push({ metric: "call_jitter", name: kind, value: jitter });
  if (packetLoss !== null)
    samples.push({ metric: "call_packet_loss", name: kind, value: Math.min(100, Math.max(0, packetLoss * 100)) });
  return samples;
}

/** Has `session` report its quality to the destination of `measureCallsWith`, as long as it lasts. */
export function reportQuality(session: RoomSession): RoomSession {
  let heard = 0;
  session.on("quality", (quality) => {
    const report = sink;
    heard += 1;
    if (report === null || (heard - 1) % qualityReportEvery !== 0) return;
    for (const sample of callSamples(session.kind, quality)) report(sample);
  });
  return session;
}
