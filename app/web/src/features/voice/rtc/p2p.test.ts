// SPDX-License-Identifier: AGPL-3.0-only
// The P2P engine with a fake RTCPeerConnection (ADR-0101 §4, §5): whom a session connects to (P1), who sends the first
// offer, glare under perfect negotiation, whose audio plays, the renewal of a grant with restartIce, the relay
// policy, and the direction of the session descriptions of a participant who does not speak. A relay between the
// connections plays the server: it passes each signal on, after checking it as the server does.
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { FakePeerConnection, FakeRemoteAudio, FakeStream, resetFakeRtc, sdpDirections, settle } from "../test/fakeRtc";
import { connectVoice, UnsupportedVoiceMethodError } from "./connect";
import { offerWait } from "./p2p";
import type { IncomingVoiceSignal, OutgoingVoiceSignal, VoiceConnection, VoiceGrant, VoiceMember } from "./types";

const grant: VoiceGrant = {
  kind: "p2p",
  options: {},
  iceServers: [{ urls: ["stun:stun.example"] }, { urls: ["turn:turn.example"], username: "u1", credential: "c1" }],
  relayOnly: false,
  expiresAt: Date.parse("2026-10-26T18:00:00Z"),
};

type Sent = OutgoingVoiceSignal & { from: string };

/** The sessions of a room, connected through a relay that checks what the server checks. */
function room() {
  const connections = new Map<string, VoiceConnection>();
  const speakers = new Set<string>();
  const sent: Sent[] = [];
  const rejected: Sent[] = [];
  const pcs = new Map<string, FakePeerConnection[]>();
  let paused = false;
  const held: (() => void)[] = [];
  const join = (session: string, options: { grant?: VoiceGrant; blocked?: boolean } = {}) => {
    const connection = connectVoice({
      session,
      grant: options.grant ?? grant,
      environment: {
        createPeerConnection: (configuration) => {
          const pc = new FakePeerConnection(configuration);
          pcs.set(session, [...(pcs.get(session) ?? []), pc]);
          return pc as unknown as RTCPeerConnection;
        },
        getMicrophone: () => Promise.resolve(new FakeStream() as unknown as MediaStream),
        createRemoteAudio: () => new FakeRemoteAudio(),
      },
      events: {
        sendSignal: (signal) => {
          const message = { ...signal, from: session };
          sent.push(message);
          // The server relays audio only, and from a participant who does not speak, receiving only.
          if ((signal.kind === "offer" || signal.kind === "answer") && !speakers.has(session)) {
            if (sdpDirections(signal.sdp ?? "").some((d) => d === "sendrecv" || d === "sendonly")) {
              rejected.push(message);
              return Promise.reject(new Error("422"));
            }
          }
          const deliver = () => {
            const incoming: IncomingVoiceSignal = { from: session, kind: signal.kind, sdp: signal.sdp, candidate: signal.candidate };
            connections.get(signal.to)?.receive(incoming);
          };
          if (paused) held.push(deliver);
          else queueMicrotask(deliver);
          return Promise.resolve();
        },
        onPlaybackBlocked: () => undefined,
      },
    });
    connections.set(session, connection);
    return connection;
  };
  /** Sends every session the state where `members` (session → speaker) are in the room. */
  const state = (members: Record<string, boolean>) => {
    speakers.clear();
    for (const [session, speaker] of Object.entries(members)) if (speaker) speakers.add(session);
    const list: VoiceMember[] = Object.entries(members).map(([session, speaker]) => ({ session, account: `a-${session}`, speaker }));
    for (const [session, connection] of connections) {
      connection.update({ speaker: members[session] ?? false, members: list.filter((m) => m.session !== session) });
    }
  };
  return {
    join,
    state,
    sent,
    rejected,
    pcs: (session: string) => pcs.get(session) ?? [],
    pause: () => {
      paused = true;
    },
    release: () => {
      paused = false;
      for (const deliver of held.splice(0)) deliver();
    },
    /** Loses the signals held so far, as a receiver's stream that does not take them in time. */
    drop: () => {
      paused = false;
      held.splice(0);
    },
  };
}

beforeEach(() => {
  resetFakeRtc();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("whom a session connects to (P1)", () => {
  test("connects a speaker to everyone, and never two listeners", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: true, b: false, c: false });
    await settle();

    expect(r.pcs("a")).toHaveLength(2);
    expect(r.pcs("b")).toHaveLength(1);
    expect(r.pcs("c")).toHaveLength(1);
    const pairs = new Set(r.sent.filter((s) => s.kind === "offer" || s.kind === "answer").map((s) => [s.from, s.to].toSorted().join("-")));
    expect(pairs).toEqual(new Set(["a-b", "a-c"]));
  });

  test("connects a listener who starts speaking to the other listeners, and closes the pair when both listen again", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: true, b: false, c: false });
    await settle();
    r.state({ a: true, b: true, c: false });
    await settle();
    expect(r.pcs("c")).toHaveLength(2);

    r.state({ a: true, b: false, c: false });
    await settle();
    const [bToA, bToC] = r.pcs("b");
    expect(bToA?.closed).toBe(false);
    expect(bToC?.closed).toBe(true);
  });

  test("closes the connection to a participant who left", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    r.state({ a: true });
    await settle();
    expect(r.pcs("a")[0]?.closed).toBe(true);
    expect(FakeRemoteAudio.all.filter((audio) => audio.closed)).toHaveLength(1);
  });
});

describe("the offers", () => {
  test("the session with the greater ID sends the first offer, and the other answers", async () => {
    const r = room();
    r.join("s1");
    r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();

    const descriptions = r.sent.filter((s) => s.kind === "offer" || s.kind === "answer");
    expect(descriptions.map((s) => `${s.from}:${s.kind}`)).toEqual(["s2:offer", "s1:answer"]);
    expect(r.pcs("s1")[0]?.signalingState).toBe("stable");
    expect(r.pcs("s2")[0]?.signalingState).toBe("stable");
    // A candidate that arrives before the description it belongs to waits for it.
    expect(r.pcs("s1")[0]?.candidates).toHaveLength(1);
    expect(r.pcs("s2")[0]?.candidates).toHaveLength(1);
  });

  test("in glare, the polite side (the smaller ID) takes the other's offer and the impolite side ignores its", async () => {
    const r = room();
    const s1 = r.join("s1");
    const s2 = r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();
    // Both take a new grant at once: each restarts ICE, and the offers cross.
    r.pause();
    s1.renew(grant);
    s2.renew(grant);
    await settle();
    expect(r.pcs("s1")[0]?.signalingState).toBe("have-local-offer");
    expect(r.pcs("s2")[0]?.signalingState).toBe("have-local-offer");
    r.release();
    await settle();

    expect(r.pcs("s1")[0]?.signalingState).toBe("stable");
    expect(r.pcs("s2")[0]?.signalingState).toBe("stable");
    const last = r.sent.filter((s) => s.kind === "answer").at(-1);
    expect(last?.from).toBe("s1");
  });

  test("a polite side without an offer asks for one with restart, and the other side offers again", async () => {
    vi.useFakeTimers();
    const r = room();
    r.join("s1");
    r.join("s2");
    // The first offer is lost.
    r.pause();
    r.state({ s1: true, s2: false });
    await settle();
    r.drop();
    expect(r.sent.filter((s) => s.kind === "offer")).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(offerWait);
    await settle();
    expect(r.sent.some((s) => s.kind === "restart" && s.from === "s1")).toBe(true);
  });

  test("restarts ICE when it fails", async () => {
    const r = room();
    r.join("s1");
    r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();
    r.pcs("s2")[0]?.failIce();
    await settle();
    expect(r.pcs("s2")[0]?.restarts).toBe(1);
    expect(r.sent.filter((s) => s.kind === "offer" && s.from === "s2")).toHaveLength(2);
  });
});

describe("the direction of a participant who does not speak", () => {
  test("a listener only receives, in its offers and its answers", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.join("z");
    r.state({ a: false, b: true, z: false });
    await settle();

    const fromListeners = r.sent.filter((s) => (s.kind === "offer" || s.kind === "answer") && s.from !== "b");
    expect(fromListeners.length).toBeGreaterThan(0);
    for (const signal of fromListeners) expect(sdpDirections(signal.sdp ?? "")).toEqual(["recvonly"]);
    expect(r.rejected).toEqual([]);
  });

  test("a speaker who listens again stops the microphone and renegotiates to receive only", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: true });
    await settle();
    await a.setMicrophone(true);
    const transceiver = r.pcs("a")[0]?.transceivers[0];
    const track = transceiver?.sender.track;
    expect(track?.enabled).toBe(true);

    r.state({ a: false, b: true });
    await settle();
    expect(track?.stopped).toBe(true);
    expect(transceiver?.sender.track).toBeNull();
    const last = r.sent.filter((s) => s.from === "a" && (s.kind === "offer" || s.kind === "answer")).at(-1);
    expect(sdpDirections(last?.sdp ?? "")).toEqual(["recvonly"]);
    expect(r.rejected).toEqual([]);
  });

  test("a listener does not turn the microphone on", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: false, b: true });
    await settle();
    await a.setMicrophone(true);
    expect(r.pcs("a")[0]?.transceivers[0]?.sender.track ?? null).toBeNull();
  });

  test("the microphone turns off without stopping, and on again", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    await a.setMicrophone(true);
    const track = r.pcs("a")[0]?.transceivers[0]?.sender.track;
    await a.setMicrophone(false);
    expect(track?.enabled).toBe(false);
    expect(track?.stopped).toBe(false);
    await a.setMicrophone(true);
    expect(track?.enabled).toBe(true);
  });
});

describe("playback", () => {
  test("plays only the audio of the participants that the latest state lists as speakers", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: false, b: true, c: true });
    await settle();
    // a hears b and c.
    const audible = () => FakeRemoteAudio.all.filter((audio) => !audio.closed && audio.audible).length;
    expect(audible()).toBe(4);

    // c listens again: nobody hears c, a and c no longer connect, and only a and c hear b.
    r.state({ a: false, b: true, c: false });
    await settle();
    expect(audible()).toBe(2);
  });

  test("tells when the browser blocks the audio, and plays it again on resume", async () => {
    FakeRemoteAudio.blocked = true;
    const onPlaybackBlocked = vi.fn();
    const connection = connectVoice({
      session: "s1",
      grant,
      environment: {
        createPeerConnection: (configuration) => new FakePeerConnection(configuration) as unknown as RTCPeerConnection,
        getMicrophone: () => Promise.resolve(new FakeStream() as unknown as MediaStream),
        createRemoteAudio: () => new FakeRemoteAudio(),
      },
      events: { sendSignal: () => Promise.resolve(), onPlaybackBlocked },
    });
    connection.update({ speaker: false, members: [{ session: "s2", account: "b", speaker: true }] });
    connection.receive({ from: "s2", kind: "offer", sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n" });
    await settle();
    expect(onPlaybackBlocked).toHaveBeenCalled();

    await connection.resumePlayback();
    expect(FakeRemoteAudio.all[0]?.playing).toBe(true);
  });
});

describe("the grant", () => {
  test("connects with the grant's ICE servers, through TURN only when it says relay", async () => {
    const r = room();
    r.join("s1", { grant: { ...grant, relayOnly: true } });
    r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();
    expect(r.pcs("s1")[0]?.configuration).toEqual({
      iceServers: [{ urls: ["stun:stun.example"] }, { urls: ["turn:turn.example"], username: "u1", credential: "c1" }],
      iceTransportPolicy: "relay",
    });
    expect(r.pcs("s2")[0]?.configuration.iceTransportPolicy).toBe("all");
  });

  test("a renewed grant goes onto every connection with setConfiguration, and restarts ICE", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: true, b: false, c: false });
    await settle();
    const renewed = { ...grant, iceServers: [{ urls: ["turn:turn.example"], username: "u2", credential: "c2" }], relayOnly: true };
    a.renew(renewed);
    await settle();

    for (const pc of r.pcs("a")) {
      expect(pc.configurations).toEqual([{ iceServers: [{ urls: ["turn:turn.example"], username: "u2", credential: "c2" }], iceTransportPolicy: "relay" }]);
      expect(pc.restarts).toBe(1);
      expect(pc.signalingState).toBe("stable");
    }
    expect(r.sent.filter((s) => s.from === "a" && s.kind === "offer" && (s.sdp ?? "").includes("ice-options:restart"))).toHaveLength(2);
  });

  test("reconnects with a new grant by connecting again", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    a.reconnect({ ...grant, relayOnly: true });
    await settle();
    expect(r.pcs("a")).toHaveLength(2);
    expect(r.pcs("a")[0]?.closed).toBe(true);
    expect(r.pcs("a")[1]?.configuration.iceTransportPolicy).toBe("relay");
  });

  test("an SFU method is not supported yet", () => {
    expect(() =>
      connectVoice({
        session: "s1",
        grant: { ...grant, kind: "livekit" },
        environment: { createPeerConnection: () => new RTCPeerConnection(), getMicrophone: () => Promise.reject(new Error("no")), createRemoteAudio: () => new FakeRemoteAudio() },
        events: { sendSignal: () => Promise.resolve(), onPlaybackBlocked: () => undefined },
      }),
    ).toThrow(UnsupportedVoiceMethodError);
  });
});

test("leaving closes every connection and stops the microphone", async () => {
  const r = room();
  const a = r.join("a");
  r.join("b");
  r.state({ a: true, b: true });
  await settle();
  await a.setMicrophone(true);
  const track = r.pcs("a")[0]?.transceivers[0]?.sender.track;
  a.close();
  expect(r.pcs("a")[0]?.closed).toBe(true);
  expect(track?.stopped).toBe(true);
});
