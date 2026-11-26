// SPDX-License-Identifier: AGPL-3.0-only
// What every RoomSession keeps, whichever transport carries it (ADR-0101 §7, §15): the package's entry makes the four,
// the relay policy of a grant, a snapshot that is a new object only when something changed, listeners that unsubscribe,
// the quality sampled at the interval asked for, and a close that happens once.
import { afterEach, beforeEach, describe, expect, jest, test } from "bun:test";
import {
  createRoomSession,
  rtcConfiguration,
  type RoomGrant,
  type RoomKind,
  type RoomRoster,
  type RoomSession,
} from "./index";
import {
  browser,
  FakePeerConnection,
  FakeSocket,
  fakeEnvironment,
  nodeOffer,
  resetFakes,
  settle,
  statsReport,
} from "./test/fake";
import { loadFakeLiveKit, resetFakeLiveKit, room } from "./test/livekit";
import { loadFakeSora, resetFakeSora, sora } from "./test/sora";

type Transport = {
  /** Joins as s1, with s2 speaking in the room, and lets the session settle. */
  start: (qualityInterval?: number) => Promise<RoomSession>;
  /** The roster the session was given at the start. */
  roster: RoomRoster;
  /** Makes the statistics of the session's media `report`. */
  stats: (report: RTCStatsReport) => void;
};

const roster: RoomRoster = { speaker: false, members: [{ session: "s2", speaker: true, participant: "a2" }] };

function grantOf(kind: RoomKind): RoomGrant {
  return {
    kind,
    url: "wss://sfu.example",
    token: "token-1",
    options: kind === "sora" ? { channel_id: "room1", role: "recvonly" } : {},
    iceServers: [{ urls: ["stun:stun.example"] }],
    relayOnly: false,
    expiresAt: Date.now() + 60 * 60_000,
  };
}

/** Starts a session of `kind` as its caller would: the grant, then the roster. */
async function started(kind: RoomKind, qualityInterval?: number): Promise<RoomSession> {
  const session = createRoomSession({
    session: "s1",
    grant: grantOf(kind),
    environment: fakeEnvironment({ loadLiveKit: loadFakeLiveKit, loadSora: loadFakeSora }),
    ...(kind === "p2p" ? { signaling: { send: () => Promise.resolve() } } : {}),
    ...(qualityInterval === undefined ? {} : { qualityInterval }),
  });
  session.update(roster);
  if (kind === "p2p") {
    // s2 speaks and has the greater ID: it offers.
    session.receiveSignal({
      from: "s2",
      kind: "offer",
      sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n",
    });
  }
  if (kind === "sfu") {
    // The node takes the WebSocket and offers s2's audio.
    FakeSocket.latest().open();
    FakeSocket.latest().receive({ v: 1, type: "offer", sdp: nodeOffer(1), tracks: [{ mid: "1", participant: "a2" }] });
  }
  await settle();
  return session;
}

const transports: Record<RoomKind, Transport> = {
  p2p: {
    start: (qualityInterval) => started("p2p", qualityInterval),
    roster,
    stats: (report) => {
      for (const pc of FakePeerConnection.all) pc.stats = report;
    },
  },
  sfu: {
    start: (qualityInterval) => started("sfu", qualityInterval),
    roster,
    stats: (report) => {
      for (const pc of FakePeerConnection.all) pc.stats = report;
    },
  },
  livekit: {
    start: (qualityInterval) => started("livekit", qualityInterval),
    roster,
    stats: (report) => {
      room().subscribe("s2", "t2", report);
    },
  },
  sora: {
    start: (qualityInterval) => started("sora", qualityInterval),
    roster,
    stats: (report) => {
      sora(0).stats = report;
    },
  },
};

/** Statistics every transport reads the same quality from: 50 ms, 4 ms of jitter, 1 % lost. */
const report = () =>
  statsReport([
    { id: "T1", type: "transport", selectedCandidatePairId: "P1" },
    { id: "P1", type: "candidate-pair", nominated: true, state: "succeeded", currentRoundTripTime: 0.05 },
    { id: "R1", type: "remote-inbound-rtp", kind: "audio", roundTripTime: 0.05 },
    { id: "I1", type: "inbound-rtp", kind: "audio", jitter: 0.004, packetsLost: 1, packetsReceived: 99 },
  ]);

beforeEach(() => {
  resetFakes();
  resetFakeLiveKit();
  resetFakeSora();
});

afterEach(() => {
  jest.useRealTimers();
});

test("the grant's ICE servers go into the configuration, through TURN only when the grant says relay", () => {
  const grant = grantOf("p2p");
  expect(rtcConfiguration({ ...grant, relayOnly: true })).toEqual({
    iceServers: [{ urls: ["stun:stun.example"] }],
    iceTransportPolicy: "relay",
  });
  expect(rtcConfiguration(grant).iceTransportPolicy).toBe("all");
});

describe.each(["p2p", "sfu", "livekit", "sora"] as const)("a %s session", (kind) => {
  const transport = transports[kind];

  test("is of the grant's kind and of the session it was made for", async () => {
    const session = await transport.start();
    expect(session.kind).toBe(kind);
    expect(session.session).toBe("s1");
  });

  test("gives the same snapshot until something changes, and a new one once it did", async () => {
    const session = await transport.start();
    const before = session.snapshot();
    expect(session.snapshot()).toBe(before);
    session.update(transport.roster);
    await settle();
    expect(session.snapshot()).toBe(before);

    session.close();
    const after = session.snapshot();
    expect(after).not.toBe(before);
    expect(after.connection.state).toBe("closed");
    expect(before.connection.state).not.toBe("closed");
    expect(session.snapshot()).toBe(after);
  });

  test("a listener that unsubscribed hears nothing more, and the others still do", async () => {
    const session = await transport.start();
    const gone: string[] = [];
    const kept: string[] = [];
    const unsubscribe = session.on("connection", (connection) => gone.push(connection.state));
    session.on("connection", (connection) => kept.push(connection.state));
    unsubscribe();
    session.close();
    expect(gone).toEqual([]);
    expect(kept).toEqual(["closed"]);
  });

  test("samples the quality at the interval it was given", async () => {
    jest.useFakeTimers();
    const session = await transport.start(1_000);
    transport.stats(report());
    const qualities: unknown[] = [];
    session.on("quality", (quality) => qualities.push(quality));
    jest.advanceTimersByTime(999);
    await settle();
    expect(qualities).toEqual([]);
    jest.advanceTimersByTime(1);
    await settle();
    expect(qualities).toEqual([{ rtt: 50, jitter: 4, packetLoss: 0.01 }]);
    session.close();
  });

  test("closing twice says closed once and releases the environment once, and the session does nothing more", async () => {
    jest.useFakeTimers();
    const session = await transport.start(1_000);
    transport.stats(report());
    const states: string[] = [];
    const qualities: unknown[] = [];
    session.on("connection", (connection) => states.push(connection.state));
    session.on("quality", (quality) => qualities.push(quality));
    session.close();
    session.close();
    expect(states).toEqual(["closed"]);
    expect(browser.closed).toBe(1);
    jest.advanceTimersByTime(5_000);
    await settle();
    expect(qualities).toEqual([]);
  });
});
