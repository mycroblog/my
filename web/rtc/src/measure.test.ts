// SPDX-License-Identifier: AGPL-3.0-only
// The quality of a call as UX samples (ADR-0167 §1, §2): the round trip and the jitter in milliseconds and the packets
// lost in percent, each named by the transport that carries the call, from a session's first sample and then one in
// `qualityReportEvery`, to the destination of `measureCallsWith` alone. A session the package's entry makes reports by
// itself; one made elsewhere through `reportQuality`.
import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import { createRoomSession, type RoomGrant, type RoomKind, type RoomSession } from "./index";
import { callSamples, measureCallsWith, qualityReportEvery, reportQuality, type CallSample } from "./measure";
import {
  FakePeerConnection,
  FakeSocket,
  fakeEnvironment,
  nodeOffer,
  resetFakes,
  settle,
  statsReport,
} from "./test/fake";

const grantOf = (kind: RoomKind): RoomGrant => ({
  kind,
  url: "wss://sfu.example",
  token: "token-1",
  iceServers: [{ urls: ["stun:stun.example"] }],
  relayOnly: false,
  expiresAt: Date.now() + 60 * 60_000,
});

/** Statistics every transport reads the same quality from: 50 ms, 4 ms of jitter, 1 % lost. */
const report = () =>
  statsReport([
    { id: "T1", type: "transport", selectedCandidatePairId: "P1" },
    { id: "P1", type: "candidate-pair", nominated: true, state: "succeeded", currentRoundTripTime: 0.05 },
    { id: "R1", type: "remote-inbound-rtp", kind: "audio", roundTripTime: 0.05 },
    { id: "I1", type: "inbound-rtp", kind: "audio", jitter: 0.004, packetsLost: 1, packetsReceived: 99 },
  ]);

/** Makes the statistics of every transport's media `stats`. */
const statsOf: Record<RoomKind, (stats: RTCStatsReport) => void> = {
  p2p: (stats) => {
    for (const pc of FakePeerConnection.all) pc.stats = stats;
  },
  sfu: (stats) => {
    for (const pc of FakePeerConnection.all) pc.stats = stats;
  },
};

/** A session of `kind` the package's entry makes, joined as s1 with s2 speaking, sampled each second. */
async function started(kind: RoomKind): Promise<RoomSession> {
  const session = createRoomSession({
    session: "s1",
    grant: grantOf(kind),
    environment: fakeEnvironment(),
    ...(kind === "p2p" ? { signaling: { send: () => Promise.resolve() } } : {}),
    qualityInterval: 1_000,
  });
  session.update({ speaker: false, members: [{ session: "s2", speaker: true, participant: "a2" }] });
  if (kind === "p2p")
    session.receiveSignal({
      from: "s2",
      kind: "offer",
      sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n",
    });
  if (kind === "sfu") {
    FakeSocket.latest().open();
    FakeSocket.latest().receive({ v: 1, type: "offer", sdp: nodeOffer(1), tracks: [{ mid: "1", participant: "a2" }] });
  }
  await settle();
  statsOf[kind](report());
  return session;
}

/** Moves the sampling on by `samples` seconds, one at a time. */
async function sampled(samples: number): Promise<void> {
  for (let i = 0; i < samples; i++) {
    jest.advanceTimersByTime(1_000);
    await settle();
  }
}

const byMetric = (samples: readonly CallSample[]) => samples.toSorted((a, b) => a.metric.localeCompare(b.metric));

/** Where the samples go in a test, until it ends. */
let heard: CallSample[] = [];
let stopHearing: (() => void) | null = null;

beforeEach(() => {
  resetFakes();
  heard = [];
  stopHearing = measureCallsWith((sample) => heard.push(sample));
});

afterEach(() => {
  stopHearing?.();
  jest.useRealTimers();
});

test("a quality gives its round trip and jitter in milliseconds and its loss in percent, and nothing it did not say", () => {
  expect(byMetric(callSamples("p2p", { rtt: 120, jitter: 8, packetLoss: 0.05 }))).toEqual([
    { metric: "call_jitter", name: "p2p", value: 8 },
    { metric: "call_packet_loss", name: "p2p", value: 5 },
    { metric: "call_rtt", name: "p2p", value: 120 },
  ]);
  expect(callSamples("sfu", { rtt: null, jitter: 3, packetLoss: null })).toEqual([
    { metric: "call_jitter", name: "sfu", value: 3 },
  ]);
  expect(callSamples("sfu", { rtt: null, jitter: null, packetLoss: null })).toEqual([]);
  expect(callSamples("sfu", { rtt: 0, jitter: 0, packetLoss: 0 })).toHaveLength(3);
});

describe.each(["p2p", "sfu"] as const)("a %s call", (kind) => {
  test("reports its quality by its transport's name, the first sample and then one in twelve", async () => {
    jest.useFakeTimers();
    const session = await started(kind);
    const quality: CallSample[] = [
      { metric: "call_jitter", name: kind, value: 4 },
      { metric: "call_packet_loss", name: kind, value: 1 },
      { metric: "call_rtt", name: kind, value: 50 },
    ];
    await sampled(1);
    expect(byMetric(heard)).toEqual(quality);

    await sampled(qualityReportEvery - 1);
    expect(heard).toHaveLength(3);

    // The same statistics again lose no packet since the previous sample, which the transport does not say.
    await sampled(1);
    expect(byMetric(heard.slice(3))).toEqual(quality.filter(({ metric }) => metric !== "call_packet_loss"));

    await sampled(qualityReportEvery - 1);
    expect(heard).toHaveLength(5);
    await sampled(1);
    expect(heard).toHaveLength(7);
    session.close();
  });

  test("reports nothing once it closed", async () => {
    jest.useFakeTimers();
    const session = await started(kind);
    session.close();
    await sampled(qualityReportEvery + 1);
    expect(heard).toEqual([]);
  });
});

test("each session counts its own samples: a second call's first sample is reported too", async () => {
  jest.useFakeTimers();
  const first = await started("sfu");
  await sampled(2);
  first.close();
  expect(heard).toHaveLength(3);

  resetFakes();
  const second = await started("sfu");
  await sampled(1);
  expect(heard).toHaveLength(6);
  second.close();
});

test("once the destination stops hearing, a call's quality goes nowhere", async () => {
  jest.useFakeTimers();
  stopHearing?.();
  stopHearing = null;
  const session = await started("p2p");
  await sampled(1);
  expect(heard).toEqual([]);
  session.close();
});

// The server's names of a call's quality are its transports' (api/spec/components/schemas/UXMetric.yaml).
test.each(["p2p", "sfu"] as const)(
  "the samples of a %s call are the server's: its metrics, its names, and values in its range",
  async (kind) => {
    jest.useFakeTimers();
    const session = await started(kind);
    await sampled(1);
    expect(heard.length).toBeGreaterThan(0);
    for (const sample of heard) {
      expect(["call_rtt", "call_jitter", "call_packet_loss"]).toContain(sample.metric);
      expect(["p2p", "sfu"]).toContain(sample.name);
      expect(sample.value).toBeGreaterThanOrEqual(0);
    }
    expect(heard.find((sample) => sample.metric === "call_packet_loss")?.value).toBeLessThanOrEqual(100);
    session.close();
  },
);

/** A session of `kind` made by hand, whose quality the test gives with `say`. */
function handMade(kind: RoomKind): { session: RoomSession; say: (rtt: number) => void } {
  const heardQuality = new Set<(quality: { rtt: number; jitter: null; packetLoss: null }) => void>();
  const session: RoomSession = {
    kind,
    session: "s1",
    capabilities: { outputSelection: false },
    update: () => undefined,
    setMicrophone: () => Promise.resolve(),
    setOutputDevice: () => Promise.resolve(),
    renew: () => undefined,
    reconnect: () => undefined,
    receiveSignal: () => undefined,
    resumePlayback: () => Promise.resolve(),
    snapshot: () => ({
      connection: { state: "connected", participants: [] },
      talking: new Set(),
      playbackBlocked: false,
      microphone: false,
      quality: null,
      failure: null,
    }),
    on: (event, listener) => {
      if (event !== "quality") return () => undefined;
      const heardOne = listener as (quality: { rtt: number; jitter: null; packetLoss: null }) => void;
      heardQuality.add(heardOne);
      return () => heardQuality.delete(heardOne);
    },
    close: () => undefined,
  };
  return {
    session,
    say: (rtt) => {
      for (const listener of heardQuality) listener({ rtt, jitter: null, packetLoss: null });
    },
  };
}

test("a session of another making reports through reportQuality as the entry's do", () => {
  const { session, say } = handMade("sfu");
  const reporting = reportQuality(session);
  expect(reporting.kind).toBe("sfu");
  say(10);
  for (let i = 0; i < qualityReportEvery - 1; i++) say(20);
  say(30);
  expect(heard).toEqual([
    { metric: "call_rtt", name: "sfu", value: 10 },
    { metric: "call_rtt", name: "sfu", value: 30 },
  ]);
});
