// SPDX-License-Identifier: AGPL-3.0-only
// The quality of the media path from WebRTC's statistics (ADR-0101 §7): the round-trip time of the selected
// candidate pair (or, without one, what the remote side reports of the sent audio), the jitter of the received
// audio, and the fraction of the received packets lost since the previous sample. Of several connections (P2P's
// pairs, an SFU's tracks), the worst round-trip time and jitter, and the loss of all of them together.
import type { ConnectionQuality } from "./types";

/** How often a session samples its quality by default. */
export const qualityInterval = 5_000;

type Stat = Record<string, unknown> & { id: string; type: string };

/** The counters of each inbound stream at the previous sample, by its connection's key and the ID of its stats. */
export type LossCounters = Map<string, { lost: number; received: number }>;

function numberOf(stat: Stat, key: string): number | undefined {
  const value = stat[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function worst(a: number | null, b: number | undefined): number | null {
  if (b === undefined) return a;
  return a === null ? b : Math.max(a, b);
}

/**
 * The quality of `reports`, each under a key of its connection (the stats' IDs are a connection's own), given the
 * counters of the previous sample, which it moves on.
 */
export function qualityOf(
  reports: readonly (readonly [string, RTCStatsReport])[],
  previous: LossCounters,
): ConnectionQuality {
  let rtt: number | null = null;
  let jitter: number | null = null;
  let lost = 0;
  let received = 0;
  const seen = new Set<string>();
  for (const [key, report] of reports) {
    const stats: Stat[] = [];
    report.forEach((stat: Stat) => {
      stats.push(stat);
    });
    const selected = new Set(
      stats.flatMap((stat) =>
        stat.type === "transport" && typeof stat.selectedCandidatePairId === "string"
          ? [stat.selectedCandidatePairId]
          : [],
      ),
    );
    let pairRtt: number | null = null;
    let remoteRtt: number | null = null;
    for (const stat of stats) {
      switch (stat.type) {
        case "candidate-pair": {
          const chosen = selected.size > 0 ? selected.has(stat.id) : stat.nominated === true;
          if (chosen && stat.state === "succeeded") pairRtt = worst(pairRtt, numberOf(stat, "currentRoundTripTime"));
          break;
        }
        case "remote-inbound-rtp":
          remoteRtt = worst(remoteRtt, numberOf(stat, "roundTripTime"));
          break;
        case "inbound-rtp": {
          if (stat.kind !== undefined && stat.kind !== "audio") break;
          jitter = worst(jitter, numberOf(stat, "jitter"));
          const nowLost = numberOf(stat, "packetsLost") ?? 0;
          const nowReceived = numberOf(stat, "packetsReceived") ?? 0;
          const counter = `${key}/${stat.id}`;
          const before = previous.get(counter) ?? { lost: 0, received: 0 };
          lost += Math.max(0, nowLost - before.lost);
          received += Math.max(0, nowReceived - before.received);
          previous.set(counter, { lost: nowLost, received: nowReceived });
          seen.add(counter);
          break;
        }
        default:
          break;
      }
    }
    rtt = worst(rtt, pairRtt ?? remoteRtt ?? undefined);
  }
  for (const id of [...previous.keys()]) if (!seen.has(id)) previous.delete(id);
  return {
    rtt: rtt === null ? null : rtt * 1000,
    jitter: jitter === null ? null : jitter * 1000,
    packetLoss: lost + received === 0 ? null : lost / (lost + received),
  };
}
