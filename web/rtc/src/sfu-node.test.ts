// SPDX-License-Identifier: AGPL-3.0-only
// The RoomSession of my's SFU on the fakes (ADR-0101 §11, §12, §14, §15): the WebSocket opened with the grant's
// ticket, the answer to the node's offer (the microphone sent on the transceiver the node receives it on, the speakers
// received), whose audio plays by the offer's tracks and the roster, who talks by the node's `speaking`, the ICE
// restarts, and the recovery through a new grant: a migration, a socket that dropped, a refused ticket, a node that
// went away, a grant that could not be had, and a version the node does not speak. The test plays the node on the
// fake WebSocket.
import { afterEach, beforeEach, describe, expect, jest, mock, test } from "bun:test";
import { disconnectWait } from "./p2p";
import { createRoomSession } from "./session";
import { talkingSample, talkingThreshold } from "./talking";
import {
  browser,
  FakeLevelMeter,
  FakePeerConnection,
  FakeRemoteAudio,
  FakeSocket,
  fakeEnvironment,
  microphone,
  nodeOffer,
  resetFakes,
  sdpDirections,
  settle,
} from "./test/fake";
import type { RoomGrant, RoomRoster, RoomSession, RoomSessionOptions } from "./types";

function grantOf(ticket: string, url = "wss://node1.example/voice/sfu"): RoomGrant {
  return {
    kind: "sfu",
    url,
    token: ticket,
    options: {},
    iceServers: [{ urls: ["turn:turn.example"], username: "u1", credential: "c1" }],
    relayOnly: false,
    expiresAt: Date.now() + 60_000,
  };
}

/** s1 listens; s2 (account a2) and s3 (account a3) speak; s4 (account a4) listens. */
const roster: RoomRoster = {
  speaker: false,
  members: [
    { session: "s2", speaker: true, participant: "a2" },
    { session: "s3", speaker: true, participant: "a3" },
    { session: "s4", speaker: false, participant: "a4" },
  ],
};

const asSpeaker: RoomRoster = { ...roster, speaker: true };

type Joined = { session: RoomSession; grants: ReturnType<typeof mock<() => Promise<RoomGrant>>> };

/** Joins as s1 with the ticket t1; each grant the session asks for has the next ticket, on node1 unless `next` says. */
function join(options: Partial<RoomSessionOptions> & { next?: () => Promise<RoomGrant> } = {}): Joined {
  let tickets = 1;
  const grants = mock(options.next ?? (() => Promise.resolve(grantOf(`t${String(++tickets)}`))));
  const session = createRoomSession({
    session: "s1",
    grant: grantOf("t1"),
    environment: fakeEnvironment(),
    requestGrant: grants,
    ...options,
  });
  session.update(roster);
  return { session, grants };
}

/** The node takes the latest WebSocket and offers the audio of `tracks` (mid → account). */
async function offered(tracks: Record<string, string>, ufrag?: string): Promise<FakeSocket> {
  const socket = FakeSocket.latest();
  if (socket.readyState === 0) socket.open();
  socket.receive({
    v: 1,
    type: "offer",
    sdp: nodeOffer(Object.keys(tracks).length, ufrag),
    tracks: Object.entries(tracks).map(([mid, participant]) => ({ mid, participant })),
  });
  await settle();
  return socket;
}

function pc(index = -1): FakePeerConnection {
  const found = FakePeerConnection.all.at(index);
  if (found === undefined) throw new Error("no connection was made");
  return found;
}

function microphoneSender(of: FakePeerConnection = pc()) {
  const transceiver = of.transceivers.find((t) => t.mid === "0");
  if (transceiver === undefined) throw new Error("no microphone transceiver");
  return transceiver;
}

/** The audios that play now, and whether each is heard. */
function playing(): boolean[] {
  return FakeRemoteAudio.all.filter((audio) => !audio.closed).map((audio) => audio.audible);
}

beforeEach(() => {
  resetFakes();
});

afterEach(() => {
  jest.useRealTimers();
});

describe("connecting", () => {
  test("opens the node's WebSocket with the ticket and the grant's ICE servers, and answers the node's offer", async () => {
    const { session } = join();
    const socket = FakeSocket.latest();
    expect(socket.url).toBe("wss://node1.example/voice/sfu?ticket=t1");
    expect(FakePeerConnection.all).toHaveLength(0);
    expect(session.snapshot().connection.state).toBe("connecting");

    await offered({ "1": "a2", "2": "a3" });
    expect(FakePeerConnection.all).toHaveLength(1);
    expect(pc().configuration.iceServers).toEqual([{ urls: ["turn:turn.example"], username: "u1", credential: "c1" }]);
    const [answer] = socket.ofType("answer");
    expect(answer?.["v"]).toBe(1);
    // The microphone's transceiver sends; the speakers' only receive. The session never offers.
    expect(sdpDirections(String(answer?.["sdp"]))).toEqual(["sendonly", "recvonly", "recvonly"]);
    expect(socket.ofType("offer")).toEqual([]);
    // Its own candidates go to the node as the JSON of an RTCIceCandidateInit.
    const [candidate] = socket.ofType("candidate");
    expect(JSON.parse(String(candidate?.["candidate"]))).toMatchObject({ sdpMid: "0", sdpMLineIndex: 0 });

    pc().setIce("connected");
    expect(session.snapshot().connection).toEqual({
      state: "connected",
      participants: [
        { session: "s2", state: "connected" },
        { session: "s3", state: "connected" },
      ],
    });
    session.close();
  });

  test("sends what it has to say once the WebSocket opens, and takes the node's candidates once it has the offer", async () => {
    const { session } = join();
    const socket = FakeSocket.latest();
    socket.receive({
      v: 1,
      type: "candidate",
      candidate: JSON.stringify({
        candidate: "candidate:1 1 udp 1 192.0.2.10 3478 typ host",
        sdpMid: "1",
        sdpMLineIndex: 1,
      }),
    });
    await settle();
    await offered({ "1": "a2" });
    expect(pc().candidates.map((c) => c.sdpMid)).toEqual(["1"]);

    // A restart asked for while the socket is connecting goes once it opens.
    socket.drop();
    await settle();
    const next = FakeSocket.latest();
    expect(next).not.toBe(socket);
    browser.networkChange();
    expect(next.sent).toEqual([]);
    next.open();
    expect(next.ofType("restart")).toEqual([{ v: 1, type: "restart" }]);
    session.close();
  });

  test("ignores a message of another version, of a type it does not know, or that is not JSON", async () => {
    const { session } = join();
    const socket = FakeSocket.latest();
    socket.open();
    socket.receive({ v: 2, type: "offer", sdp: nodeOffer(1), tracks: [] });
    socket.receive({ v: 1, type: "hello" });
    socket.receive("not json");
    await settle();
    expect(FakePeerConnection.all).toHaveLength(0);
    expect(session.snapshot().connection.state).toBe("connecting");
    session.close();
  });
});

describe("the audio", () => {
  test("plays each listed transceiver while the roster lists its participant as a speaker", async () => {
    const { session } = join();
    await offered({ "1": "a2", "2": "a4" });
    // a4 does not speak: its audio is not heard, whatever the node sends.
    expect(playing()).toEqual([true, false]);

    session.update({ ...roster, members: roster.members.map((m) => ({ ...m, speaker: m.session !== "s2" })) });
    expect(playing()).toEqual([false, true]);
    session.close();
    expect(playing()).toEqual([]);
  });

  test("a transceiver the next offer no longer lists stops, and one that carries another speaker plays them", async () => {
    const { session } = join();
    await offered({ "1": "a2", "2": "a3" });
    const [first, second] = FakeRemoteAudio.all;
    await offered({ "1": "a3" });
    expect(first?.closed).toBe(true);
    expect(second?.closed).toBe(true);
    expect(playing()).toEqual([true]);
    expect(session.snapshot().connection.participants.map((p) => p.session)).toEqual(["s3"]);
    session.close();
  });

  test("a participant the roster names after the offer plays once it does", async () => {
    const { session } = join();
    session.update({ speaker: false, members: [] });
    await offered({ "1": "a2" });
    expect(playing()).toEqual([false]);
    session.update(roster);
    expect(playing()).toEqual([true]);
    session.close();
  });
});

describe("who talks", () => {
  test("is what the node's speaking says of the roster's speakers, by their sessions", async () => {
    const { session } = join();
    await offered({ "1": "a2", "2": "a3" });
    const heard: string[][] = [];
    session.on("talking", (sessions) => heard.push([...sessions].sort()));
    FakeSocket.latest().receive({ v: 1, type: "speaking", participants: ["a2", "a4", "a9"] });
    FakeSocket.latest().receive({ v: 1, type: "speaking", participants: ["a2", "a3"] });
    FakeSocket.latest().receive({ v: 1, type: "speaking", participants: [] });
    expect(heard).toEqual([["s2"], ["s2", "s3"], []]);
    session.close();
  });

  test("the session itself talks while its microphone sends and its level says so", async () => {
    jest.useFakeTimers();
    const { session } = join();
    session.update(asSpeaker);
    await offered({ "1": "a2" });
    await session.setMicrophone(true);
    FakeLevelMeter.of(microphone.latest()).value = talkingThreshold;
    jest.advanceTimersByTime(talkingSample);
    expect([...session.snapshot().talking]).toEqual(["s1"]);
    session.close();
  });
});

describe("the microphone", () => {
  test("goes on the node's transceiver while the session speaks, without a new answer, and off when it stops", async () => {
    const { session } = join();
    session.update(asSpeaker);
    const socket = await offered({ "1": "a2" });
    await session.setMicrophone(true);
    const track = microphone.latest().tracks[0] ?? null;
    expect(microphoneSender().sender.track).toBe(track);
    expect(session.snapshot().microphone).toBe(true);
    expect(socket.ofType("answer")).toHaveLength(1);

    await session.setMicrophone(false);
    expect(track?.enabled).toBe(false);
    expect(session.snapshot().microphone).toBe(false);

    await session.setMicrophone(true);
    session.update(roster);
    await settle();
    expect(microphoneSender().sender.track).toBeNull();
    expect(track?.stopped).toBe(true);
    expect(session.snapshot().microphone).toBe(false);
    session.close();
  });

  test("turned on before the offer, it is sent on the transceiver the offer makes", async () => {
    const { session } = join();
    session.update(asSpeaker);
    await session.setMicrophone(true);
    await offered({});
    expect(microphoneSender().sender.track).toBe(microphone.latest().tracks[0] ?? null);
    session.close();
  });
});

describe("ICE", () => {
  test("asks the node for a restart when ICE fails, stays disconnected, or the network changes", async () => {
    jest.useFakeTimers();
    const { session } = join();
    const socket = await offered({ "1": "a2" });
    pc().setIce("connected");
    pc().setIce("failed");
    expect(session.snapshot().connection.state).toBe("reconnecting");
    expect(socket.ofType("restart")).toHaveLength(1);

    pc().setIce("disconnected");
    jest.advanceTimersByTime(disconnectWait - 1);
    expect(socket.ofType("restart")).toHaveLength(1);
    jest.advanceTimersByTime(1);
    expect(socket.ofType("restart")).toHaveLength(2);

    browser.networkChange();
    expect(socket.ofType("restart")).toHaveLength(3);

    // The node's restart offer is answered on the same connection.
    await offered({ "1": "a2" }, "node2");
    expect(FakePeerConnection.all).toHaveLength(1);
    expect(socket.ofType("answer")).toHaveLength(2);
    session.close();
  });

  test("a renewed grant's ICE servers go onto the connection", async () => {
    const { session } = join();
    await offered({ "1": "a2" });
    const renewed = { ...grantOf("t9"), iceServers: [{ urls: ["turn:turn2.example"] }] };
    session.renew(renewed);
    expect(pc().configurations.at(-1)?.iceServers).toEqual([{ urls: ["turn:turn2.example"] }]);
    expect(FakeSocket.all).toHaveLength(1);
    session.close();
  });
});

describe("recovering", () => {
  test("a socket that drops while the connection lives connects again with a new grant on the same connection", async () => {
    const { session, grants } = join();
    const socket = await offered({ "1": "a2" });
    pc().setIce("connected");
    socket.drop();
    await settle();
    expect(grants).toHaveBeenCalledTimes(1);
    expect(FakeSocket.latest().ticket).toBe("t2");
    // The media path did not change: the audio goes on, and the node reattaches the session.
    expect(session.snapshot().connection.state).toBe("connected");
    FakeSocket.latest().open();
    expect(FakePeerConnection.all).toHaveLength(1);
    expect(pc().closed).toBe(false);
    session.close();
  });

  test("migrate moves the session to the node of a new grant, on a new connection", async () => {
    const grantTwo = grantOf("t2", "wss://node2.example/voice/sfu");
    const { session, grants } = join({ next: () => Promise.resolve(grantTwo) });
    const socket = await offered({ "1": "a2" });
    pc().setIce("connected");
    socket.receive({ v: 1, type: "migrate" });
    socket.drop(1000);
    await settle();
    expect(grants).toHaveBeenCalledTimes(1);
    expect(FakeSocket.latest().url).toBe("wss://node2.example/voice/sfu?ticket=t2");
    expect(pc(0).closed).toBe(true);
    expect(playing()).toEqual([]);
    expect(session.snapshot().connection.state).toBe("reconnecting");

    await offered({ "1": "a2" });
    expect(FakePeerConnection.all).toHaveLength(2);
    expect(FakeSocket.latest().ofType("answer")).toHaveLength(1);
    pc().setIce("connected");
    expect(session.snapshot().connection.state).toBe("connected");
    expect(playing()).toEqual([true]);
    session.close();
  });

  test("a refused ticket keeps the connection; a room no longer on the node, or a node gone, does not", async () => {
    const { session, grants } = join();
    await offered({ "1": "a2" });
    pc().setIce("connected");

    FakeSocket.latest().receive({ v: 1, type: "error", code: "invalid_ticket", message: "" });
    FakeSocket.latest().drop(1008);
    await settle();
    expect(pc(0).closed).toBe(false);
    await offered({ "1": "a2" });

    FakeSocket.latest().receive({ v: 1, type: "error", code: "something_new", message: "" });
    FakeSocket.latest().drop(1000);
    await settle();
    expect(pc(0).closed).toBe(true);
    await offered({ "1": "a2" });
    expect(FakePeerConnection.all).toHaveLength(2);

    // The socket closed and ICE failed: the node is gone.
    pc().setIce("connected");
    FakeSocket.latest().drop();
    pc().setIce("failed");
    await settle();
    expect(pc(1).closed).toBe(true);
    await offered({ "1": "a2" });
    expect(FakePeerConnection.all).toHaveLength(3);
    expect(grants).toHaveBeenCalledTimes(3);
    session.close();
  });

  test("a grant that cannot be had says lost, and the caller's reconnect connects again", async () => {
    const { session, grants } = join({ next: () => Promise.reject(new Error("offline")) });
    await offered({ "1": "a2" });
    pc().setIce("connected");
    FakeSocket.latest().drop();
    await settle();
    expect(grants).toHaveBeenCalledTimes(1);
    expect(session.snapshot().connection).toEqual({ state: "lost", participants: [] });

    session.reconnect(grantOf("t5"));
    expect(FakeSocket.latest().ticket).toBe("t5");
    expect(session.snapshot().connection.state).toBe("connected");
    session.close();
  });

  test("a new socket that closes again before an offer or ICE came back says lost without asking again", async () => {
    const { session, grants } = join();
    await offered({ "1": "a2" });
    FakeSocket.latest().drop();
    await settle();
    FakeSocket.latest().open();
    FakeSocket.latest().drop();
    await settle();
    expect(grants).toHaveBeenCalledTimes(1);
    expect(session.snapshot().connection.state).toBe("lost");
    // While lost, a network change waits for the caller.
    browser.networkChange();
    expect(FakeSocket.all).toHaveLength(2);
    session.close();
  });

  test("without a way to a new grant, a socket that drops says lost", async () => {
    const session = createRoomSession({ session: "s1", grant: grantOf("t1"), environment: fakeEnvironment() });
    FakeSocket.latest().drop();
    await settle();
    expect(session.snapshot().connection.state).toBe("lost");
    session.close();
  });

  test("a grant asked for before the caller's reconnect is dropped when it comes", async () => {
    let give: (grant: RoomGrant) => void = () => undefined;
    const { session } = join({ next: () => new Promise((resolve) => (give = resolve)) });
    FakeSocket.latest().drop();
    await settle();
    session.reconnect(grantOf("t7"));
    give(grantOf("t8"));
    await settle();
    expect(FakeSocket.all.map((socket) => socket.ticket)).toEqual(["t1", "t7"]);
    session.close();
  });

  test("a version the node does not speak fails the session as unavailable", async () => {
    const { session } = join();
    const failures: string[] = [];
    session.on("failed", (failure) => failures.push(failure.reason));
    FakeSocket.latest().open();
    FakeSocket.latest().receive({ v: 1, type: "error", code: "unsupported_version", message: "" });
    FakeSocket.latest().drop(1008);
    await settle();
    expect(failures).toEqual(["unavailable"]);
    expect(session.snapshot().connection.state).toBe("closed");
  });
});

test("closing closes the WebSocket, the connection and the audio, and asks for no grant after", async () => {
  const { session, grants } = join();
  const socket = await offered({ "1": "a2" });
  session.close();
  await settle();
  expect(socket.closedWith).toBe(1000);
  expect(pc().closed).toBe(true);
  expect(playing()).toEqual([]);
  expect(browser.closed).toBe(1);
  expect(grants).not.toHaveBeenCalled();
});
