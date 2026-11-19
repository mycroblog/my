// SPDX-License-Identifier: AGPL-3.0-only
// The P2P RoomSession on the fakes (ADR-0101 §4, §5): whom a session connects to, who sends the first offer, glare
// under perfect negotiation, whose audio plays, the renewal of a grant with restartIce, the relay policy, the
// direction of the descriptions of a participant who does not speak, the recovery from lost or refused signals, ICE
// restarts on a failure and a network change, the microphone taken again when its device goes away, and what the
// session tells. A relay between the sessions plays the server: it passes each signal on, after checking it as the
// server does.
import { afterEach, beforeEach, describe, expect, jest, mock, test } from "bun:test";
import { disconnectWait, offerWait, resendWait } from "./p2p";
import { createRoomSession, UnsupportedRoomKindError } from "./session";
import { talkingSample, talkingThreshold } from "./talking";
import {
  browser,
  FakeLevelMeter,
  FakePeerConnection,
  FakeRemoteAudio,
  FakeStream,
  fakeEnvironment,
  microphone,
  rejection,
  resetFakes,
  sdpDirections,
  settle,
  statsReport,
} from "./test/fake";
import type { IncomingSignal, OutgoingSignal, RoomEnvironment, RoomGrant, RoomMember, RoomSession } from "./types";

const grant: RoomGrant = {
  kind: "p2p",
  options: {},
  iceServers: [{ urls: ["stun:stun.example"] }, { urls: ["turn:turn.example"], username: "u1", credential: "c1" }],
  relayOnly: false,
  expiresAt: Date.parse("2026-10-26T18:00:00Z"),
};

type Sent = OutgoingSignal & { from: string };

/** The sessions of a room, connected through a relay that checks what the server checks. */
function room() {
  const sessions = new Map<string, RoomSession>();
  const speakers = new Set<string>();
  const sent: Sent[] = [];
  const rejected: Sent[] = [];
  const pcs = new Map<string, FakePeerConnection[]>();
  const meters = new Map<string, FakeLevelMeter[]>();
  const talking = new Map<string, string[]>();
  let paused = false;
  const held: (() => void)[] = [];
  /** The signals that the relay refuses (a 422 or a lost request). */
  let refuse: ((signal: Sent) => boolean) | null = null;
  const join = (session: string, options: { grant?: RoomGrant; environment?: Partial<RoomEnvironment> } = {}) => {
    const joined = createRoomSession({
      session,
      grant: options.grant ?? grant,
      environment: fakeEnvironment({
        createPeerConnection: (configuration) => {
          const pc = new FakePeerConnection(configuration);
          pcs.set(session, [...(pcs.get(session) ?? []), pc]);
          return pc as unknown as RTCPeerConnection;
        },
        createLevelMeter: (stream) => {
          const meter = new FakeLevelMeter(stream);
          meters.set(session, [...(meters.get(session) ?? []), meter]);
          return meter;
        },
        ...options.environment,
      }),
      signaling: {
        send: (signal) => {
          const message = { ...signal, from: session };
          sent.push(message);
          if (refuse?.(message) === true) {
            rejected.push(message);
            return Promise.reject(new Error("refused"));
          }
          // The server relays audio only, and from a participant who does not speak, receiving only.
          if ((signal.kind === "offer" || signal.kind === "answer") && !speakers.has(session)) {
            if (sdpDirections(signal.sdp ?? "").some((d) => d === "sendrecv" || d === "sendonly")) {
              rejected.push(message);
              return Promise.reject(new Error("422"));
            }
          }
          const deliver = () => {
            const incoming: IncomingSignal = {
              from: session,
              kind: signal.kind,
              sdp: signal.sdp,
              candidate: signal.candidate,
            };
            sessions.get(signal.to)?.receiveSignal(incoming);
          };
          if (paused) held.push(deliver);
          else queueMicrotask(deliver);
          return Promise.resolve();
        },
      },
    });
    joined.on("talking", (now) => {
      talking.set(session, [...now].sort());
    });
    sessions.set(session, joined);
    return joined;
  };
  /** Gives every session the roster where `members` (session → speaker) are in the room. */
  const state = (members: Record<string, boolean>) => {
    speakers.clear();
    for (const [session, speaker] of Object.entries(members)) if (speaker) speakers.add(session);
    const list: RoomMember[] = Object.entries(members).map(([session, speaker]) => ({ session, speaker }));
    for (const [session, joined] of sessions) {
      joined.update({ speaker: members[session] ?? false, members: list.filter((m) => m.session !== session) });
    }
  };
  return {
    join,
    state,
    sent,
    rejected,
    pcs: (session: string) => pcs.get(session) ?? [],
    /** The open level meters of `session`. */
    meters: (session: string) => (meters.get(session) ?? []).filter((meter) => !meter.closed),
    talking: (session: string) => talking.get(session) ?? [],
    refuse: (which: ((signal: Sent) => boolean) | null) => {
      refuse = which;
    },
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

/** Moves the fake clock on by `ms` and lets what it started run. */
async function advance(ms: number): Promise<void> {
  jest.advanceTimersByTime(ms);
  await settle();
}

beforeEach(() => {
  resetFakes();
});

afterEach(() => {
  jest.useRealTimers();
});

describe("whom a session connects to (ADR-0101 §4)", () => {
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
    const pairs = new Set(
      r.sent.filter((s) => s.kind === "offer" || s.kind === "answer").map((s) => [s.from, s.to].sort().join("-")),
    );
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
  });

  test("in glare, the polite side (the smaller ID) takes the other's offer and the impolite side ignores its", async () => {
    const r = room();
    const s1 = r.join("s1");
    const s2 = r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();
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
    expect(r.sent.filter((s) => s.kind === "answer").at(-1)?.from).toBe("s1");
  });

  test("a polite side without an offer asks for one with restart", async () => {
    jest.useFakeTimers();
    const r = room();
    r.join("s1");
    r.join("s2");
    r.pause();
    r.state({ s1: true, s2: false });
    await settle();
    r.drop();
    await advance(offerWait);
    expect(r.sent.some((s) => s.kind === "restart" && s.from === "s1")).toBe(true);
  });
});

describe("stability", () => {
  test("restarts ICE when it fails", async () => {
    const r = room();
    r.join("s1");
    r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();
    r.pcs("s2")[0]?.setIce("failed");
    await settle();
    expect(r.pcs("s2")[0]?.restarts).toBe(1);
    expect(r.sent.filter((s) => s.kind === "offer" && s.from === "s2")).toHaveLength(2);
  });

  test("restarts ICE when it stays disconnected, and not when it comes back by itself", async () => {
    jest.useFakeTimers();
    const r = room();
    r.join("s1");
    r.join("s2");
    r.state({ s1: true, s2: true });
    await settle();
    const pc = r.pcs("s2")[0];
    pc?.setIce("disconnected");
    pc?.setIce("connected");
    await advance(disconnectWait);
    expect(pc?.restarts).toBe(0);

    pc?.setIce("disconnected");
    await advance(disconnectWait);
    expect(pc?.restarts).toBe(1);
  });

  test("restarts ICE on every pair when the network changes", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: true, b: false, c: false });
    await settle();
    browser.networkChange();
    await settle();
    expect(r.pcs("a").map((pc) => pc.restarts)).toEqual([1, 1]);
    expect(r.pcs("c").map((pc) => pc.restarts)).toEqual([1]);
  });

  test("takes the microphone again when its track ends, and sends the new track on every pair", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: true, b: false, c: false });
    await settle();
    await a.setMicrophone(true);
    const first = microphone.latest().tracks[0];

    microphone.device = "mic-2";
    first?.end();
    await settle();
    const next = microphone.latest().tracks[0];
    expect(next?.deviceId).toBe("mic-2");
    expect(first?.stopped).toBe(true);
    for (const pc of r.pcs("a")) expect(pc.transceivers[0]?.sender.track).toBe(next ?? null);
  });

  test("takes the microphone again after a devicechange whose inputs no longer have its device", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    await a.setMicrophone(true);
    const first = microphone.latest().tracks[0];
    microphone.inputs = ["mic-1"];
    browser.deviceChange();
    await settle();
    expect(microphone.streams).toHaveLength(1);

    microphone.device = "mic-2";
    microphone.inputs = ["mic-2"];
    browser.deviceChange();
    await settle();
    expect(microphone.streams).toHaveLength(2);
    expect(first?.stopped).toBe(true);
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
    expect(a.snapshot().microphone).toBe(true);

    r.state({ a: false, b: true });
    await settle();
    expect(track?.stopped).toBe(true);
    expect(transceiver?.sender.track).toBeNull();
    expect(a.snapshot().microphone).toBe(false);
    const last = r.sent.filter((s) => s.from === "a" && (s.kind === "offer" || s.kind === "answer")).at(-1);
    expect(sdpDirections(last?.sdp ?? "")).toEqual(["recvonly"]);
    expect(r.rejected).toEqual([]);
  });

  test("turning the microphone on rejects when the browser refuses it, and sends nothing", async () => {
    microphone.refused = true;
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    expect(await rejection(a.setMicrophone(true))).toMatchObject({ name: "NotAllowedError" });
    expect(a.snapshot().microphone).toBe(false);
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

  test("the microphone's changes apply one after another: one request for it, and the latest wish wins", async () => {
    let give: (stream: MediaStream) => void = () => undefined;
    const getMicrophone = mock(
      () =>
        new Promise<MediaStream>((resolve) => {
          give = resolve;
        }),
    );
    const r = room();
    const a = r.join("a", { environment: { getMicrophone } });
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    const first = a.setMicrophone(true);
    await settle();
    // While the browser asks: on again, then off.
    const changes = [first, a.setMicrophone(true), a.setMicrophone(false)];
    await settle();
    const stream = new FakeStream();
    give(stream as unknown as MediaStream);
    await Promise.all(changes);
    expect(getMicrophone).toHaveBeenCalledTimes(1);
    expect(stream.tracks[0]?.enabled).toBe(false);
  });
});

describe("who is talking", () => {
  test("a remote speaker who is heard, and the user while the microphone sends", async () => {
    jest.useFakeTimers();
    const r = room();
    const a = r.join("a");
    const b = r.join("b");
    r.state({ a: true, b: true });
    await settle();
    expect(r.meters("a")).toHaveLength(1);
    expect(r.meters("b")).toHaveLength(1);

    await a.setMicrophone(true);
    const sent = r.pcs("a")[0]?.transceivers[0]?.sender.track;
    const own = r.meters("a").find((meter) => (meter.stream as FakeStream).tracks.some((track) => track === sent));
    if (own === undefined) throw new Error("no meter of the microphone");
    own.value = talkingThreshold * 5;
    await advance(talkingSample);
    expect(r.talking("a")).toEqual(["a"]);
    expect([...a.snapshot().talking]).toEqual(["a"]);

    for (const meter of r.meters("b")) meter.value = talkingThreshold * 5;
    await advance(talkingSample);
    expect(r.talking("b")).toEqual(["a"]);

    await a.setMicrophone(false);
    expect(r.talking("a")).toEqual([]);
    expect(own.closed).toBe(true);

    r.state({ a: false, b: true });
    await settle();
    expect(r.talking("b")).toEqual([]);

    a.close();
    b.close();
    expect(FakeLevelMeter.open()).toEqual([]);
  });
});

describe("playback", () => {
  test("plays only the audio of the participants that the latest roster lists as speakers", async () => {
    const r = room();
    r.join("a");
    r.join("b");
    r.join("c");
    r.state({ a: false, b: true, c: true });
    await settle();
    const audible = () => FakeRemoteAudio.all.filter((audio) => !audio.closed && audio.audible).length;
    expect(audible()).toBe(4);

    r.state({ a: false, b: true, c: false });
    await settle();
    expect(audible()).toBe(2);
  });

  test("a participant who leaves the roster loses its pair and its audio", async () => {
    const r = room();
    const heard: FakeRemoteAudio[] = [];
    const a = r.join("a", {
      environment: {
        createRemoteAudio: () => {
          const audio = new FakeRemoteAudio();
          heard.push(audio);
          return audio;
        },
      },
    });
    r.join("b");
    r.join("c");
    r.state({ a: true, b: true, c: false });
    await settle();
    const [toB, toC] = r.pcs("a");
    // Of b and c, only b sends audio to a.
    const fromB = heard.filter((audio) => !audio.closed && audio.audible);
    expect(fromB).toHaveLength(1);

    a.update({ speaker: true, members: [{ session: "c", speaker: false }] });
    await settle();
    expect(toB?.closed).toBe(true);
    expect(toC?.closed).toBe(false);
    expect(fromB.map((audio) => audio.closed)).toEqual([true]);
    expect(a.snapshot().connection.participants.map((p) => p.session)).toEqual(["c"]);
  });

  test("says when the browser blocks the audio, and that it plays again on resume", async () => {
    FakeRemoteAudio.blocked = true;
    const session = createRoomSession({
      session: "s1",
      grant,
      environment: fakeEnvironment(),
      signaling: { send: () => Promise.resolve() },
    });
    const blocked: boolean[] = [];
    session.on("playbackBlocked", (now) => blocked.push(now));
    session.update({ speaker: false, members: [{ session: "s2", speaker: true }] });
    session.receiveSignal({
      from: "s2",
      kind: "offer",
      sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n",
    });
    await settle();
    expect(blocked).toEqual([true]);
    expect(session.snapshot().playbackBlocked).toBe(true);

    expect(await rejection(session.resumePlayback())).toMatchObject({ name: "NotAllowedError" });
    FakeRemoteAudio.blocked = false;
    await session.resumePlayback();
    expect(blocked).toEqual([true, false]);
    expect(FakeRemoteAudio.all[0]?.playing).toBe(true);
  });

  test("chooses the output device where the environment can, and says so", async () => {
    const r = room();
    const a = r.join("a");
    expect(a.capabilities.outputSelection).toBe(true);
    await a.setOutputDevice("speaker-2");
    expect(browser.outputDevices).toEqual(["speaker-2"]);

    const b = r.join("b", { environment: { setOutputDevice: undefined } });
    expect(b.capabilities.outputSelection).toBe(false);
    expect(await rejection(b.setOutputDevice("speaker-2"))).toBeInstanceOf(Error);
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
    const renewed = { ...grant, iceServers: [{ urls: ["turn:turn.example"], username: "u2", credential: "c2" }] };
    a.renew({ ...renewed, relayOnly: true });
    await settle();
    for (const pc of r.pcs("a")) {
      expect(pc.configurations).toEqual([
        {
          iceServers: [{ urls: ["turn:turn.example"], username: "u2", credential: "c2" }],
          iceTransportPolicy: "relay",
        },
      ]);
      expect(pc.restarts).toBe(1);
      expect(pc.signalingState).toBe("stable");
    }
  });

  test("connects the pair again where the browser refuses the new configuration", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    FakePeerConnection.refusesConfiguration = true;
    a.renew({ ...grant, relayOnly: true });
    await settle();
    expect(r.pcs("a")).toHaveLength(2);
    expect(r.pcs("a")[0]?.closed).toBe(true);
    expect(r.pcs("a")[1]?.configuration.iceTransportPolicy).toBe("relay");
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

  test("a kind it does not know, or P2P without its signaling port, is refused", () => {
    expect(() =>
      createRoomSession({ session: "s1", grant: { ...grant, kind: "mystery" }, environment: fakeEnvironment() }),
    ).toThrow(UnsupportedRoomKindError);
    expect(() => createRoomSession({ session: "s1", grant, environment: fakeEnvironment() })).toThrow(
      UnsupportedRoomKindError,
    );
  });
});

describe("lost and refused signals", () => {
  test("an offer whose answer is lost is rolled back and made again", async () => {
    jest.useFakeTimers();
    const r = room();
    r.join("s1");
    r.join("s2");
    r.refuse((s) => s.kind === "answer");
    r.state({ s1: true, s2: true });
    await settle();
    expect(r.pcs("s2")[0]?.signalingState).toBe("have-local-offer");
    r.refuse(null);
    await advance(offerWait);
    await advance(0);
    expect(r.pcs("s1")[0]?.signalingState).toBe("stable");
    expect(r.pcs("s2")[0]?.signalingState).toBe("stable");
  });

  test("an offer that the relay refuses is made again a little later", async () => {
    jest.useFakeTimers();
    const r = room();
    r.join("s1");
    r.join("s2");
    r.refuse((s) => s.kind === "offer" && r.rejected.length === 0);
    r.state({ s1: true, s2: true });
    await settle();
    expect(r.rejected).toHaveLength(1);
    await advance(resendWait);
    expect(r.pcs("s1")[0]?.signalingState).toBe("stable");
    expect(r.pcs("s2")[0]?.signalingState).toBe("stable");
  });

  test("a candidate of another ICE generation, queued before the offer, is dropped and the offer answered", async () => {
    const r = room();
    const s1 = r.join("s1");
    r.state({ s1: false, s2: true });
    await settle();
    const candidate = (usernameFragment: string) =>
      JSON.stringify({
        candidate: "candidate:9 1 udp 1 192.0.2.9 9 typ host",
        sdpMid: "0",
        sdpMLineIndex: 0,
        usernameFragment,
      });
    s1.receiveSignal({ from: "s2", kind: "candidate", candidate: candidate("old") });
    s1.receiveSignal({
      from: "s2",
      kind: "offer",
      sdp: "v=0\r\na=ice-ufrag:new\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n",
    });
    s1.receiveSignal({ from: "s2", kind: "candidate", candidate: candidate("new") });
    await settle();
    expect(r.pcs("s1")[0]?.candidates.map((c) => c.usernameFragment)).toEqual(["new"]);
    expect(r.sent.filter((s) => s.kind === "answer")).toHaveLength(1);
  });

  test("ignores an offer from a listener while the session listens too (a stale one)", async () => {
    const r = room();
    const s1 = r.join("s1");
    r.state({ s1: false, s2: false });
    s1.receiveSignal({ from: "s2", kind: "offer", sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=recvonly\r\n" });
    await settle();
    expect(r.pcs("s1")).toEqual([]);
  });
});

describe("what the session tells", () => {
  test("the connection of each pair, from its ICE", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    const seen: string[] = [];
    a.on("connection", (connection) => seen.push(connection.state));
    expect(a.snapshot().connection).toEqual({ state: "connected", participants: [] });
    r.state({ a: true, b: false });
    await settle();
    expect(a.snapshot().connection).toEqual({
      state: "connecting",
      participants: [{ session: "b", state: "connecting" }],
    });
    r.pcs("a")[0]?.setIce("connected");
    expect(a.snapshot().connection).toEqual({
      state: "connected",
      participants: [{ session: "b", state: "connected" }],
    });
    r.pcs("a")[0]?.setIce("disconnected");
    expect(a.snapshot().connection.state).toBe("reconnecting");
    expect(seen).toEqual(["connecting", "connected", "reconnecting"]);
  });

  test("the quality, from the statistics of every pair", async () => {
    jest.useFakeTimers();
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: false });
    await settle();
    const pc = r.pcs("a")[0];
    if (pc === undefined) throw new Error("no connection");
    const qualities: unknown[] = [];
    a.on("quality", (quality) => qualities.push(quality));
    const sample = (lost: number, received: number) =>
      statsReport([
        { id: "T1", type: "transport", selectedCandidatePairId: "P1" },
        { id: "P1", type: "candidate-pair", state: "succeeded", currentRoundTripTime: 0.12 },
        { id: "P2", type: "candidate-pair", state: "succeeded", currentRoundTripTime: 0.9 },
        { id: "I1", type: "inbound-rtp", kind: "audio", jitter: 0.015, packetsLost: lost, packetsReceived: received },
      ]);
    pc.stats = sample(10, 90);
    await advance(5_000);
    pc.stats = sample(15, 185);
    await advance(5_000);
    expect(qualities).toEqual([
      { rtt: 120, jitter: 15, packetLoss: 0.1 },
      { rtt: 120, jitter: 15, packetLoss: 0.05 },
    ]);
  });

  test("closing closes every connection, stops the microphone and releases the environment, once", async () => {
    const r = room();
    const a = r.join("a");
    r.join("b");
    r.state({ a: true, b: true });
    await settle();
    await a.setMicrophone(true);
    const track = r.pcs("a")[0]?.transceivers[0]?.sender.track;
    const states: string[] = [];
    a.on("connection", (connection) => states.push(connection.state));
    a.close();
    a.close();
    expect(r.pcs("a")[0]?.closed).toBe(true);
    expect(track?.stopped).toBe(true);
    expect(FakeRemoteAudio.all.filter((audio) => !audio.closed)).toHaveLength(1);
    expect(browser.closed).toBe(1);
    expect(states).toEqual(["closed"]);
    a.update({ speaker: true, members: [{ session: "c", speaker: true }] });
    expect(r.pcs("a")).toHaveLength(1);
  });
});
