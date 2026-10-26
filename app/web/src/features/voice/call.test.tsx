// SPDX-License-Identifier: AGPL-3.0-only
// The call of a room's page through the whole app (web-client/README.md T4; ADR-0100 §5, §6, §10; ADR-0101 §3–§7):
// MSW plays the server and its room stream, and a fake RTCPeerConnection the browser's WebRTC. Joining opens the
// stream with the session and sends heartbeats; the user answers the host's offer receiving only; the microphones,
// reactions, and raised hands of the participants show; a user who becomes a speaker turns the microphone on, and a
// host's mute turns it off; leaving asks the host for the next host; the host's menu operates on a participant; and
// the end of the room stays on the page, which says the room ended once the server no longer has it; and the host who
// starts a room is in its call at once.
import { act, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { instanceFixture, otherAccountFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, oauthPaths, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { FakeAudioContext, FakePeerConnection, FakeStream, resetFakeRtc, sdpDirections } from "./test/fakeRtc";
import { participantFixture, roomId, voiceRoomFixture } from "./test/handlers";

const grant: Schemas["VoiceGrant"] = { kind: "p2p", options: {}, ice_servers: [{ urls: ["stun:stun.example"] }], relay_only: false, expires_at: "2099-01-01T00:00:00Z" };

const encoder = new TextEncoder();

/** The server with voice chat on and `room` open, its room stream, and the requests the app sends. */
function serve(room: Schemas["VoiceRoom"], joined: Schemas["VoiceRoom"]) {
  let stream: ReadableStreamDefaultController<Uint8Array> | undefined;
  const sessions: string[] = [];
  const requests: { method: string; path: string; body: unknown }[] = [];
  let current = room;
  let ended = false;
  const record = async (request: Request) => {
    const url = new URL(request.url);
    const text = request.method === "GET" || request.method === "DELETE" ? "" : await request.text();
    requests.push({ method: request.method, path: `${url.pathname.slice(apiBase.length)}${url.search}`, body: text === "" ? undefined : JSON.parse(text) });
  };
  const respondRoom = (next: Schemas["VoiceRoom"]) => {
    current = next;
    return HttpResponse.json(next);
  };
  server.use(
    signedInHandler,
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, voice_enabled: true, voice_room_starters: "everyone" })),
    http.get(`${apiBase}/voice/rooms/:id`, ({ request }) =>
      withViewer(request, () => (ended ? HttpResponse.json({ type: "about:blank", title: "Not Found", status: 404 }, { status: 404 }) : HttpResponse.json(current))),
    ),
    http.get(`${apiBase}/voice/rooms/:id/summary`, () => HttpResponse.json({ id: current.id, state: ended ? "ended" : "open", name: current.name })),
    http.post(`${apiBase}/voice/rooms/:id/participation`, async ({ request }) => {
      await record(request);
      return withViewer(request, () => {
        current = joined;
        return HttpResponse.json({ room: joined, session: "s1", grant });
      });
    }),
    http.delete(`${apiBase}/voice/rooms/:id/participation`, async ({ request }) => {
      await record(request);
      current = room;
      return new HttpResponse(null, { status: 204 });
    }),
    http.post(`${apiBase}/voice/rooms/:id/heartbeat`, async ({ request }) => {
      await record(request);
      return new HttpResponse(null, { status: 204 });
    }),
    http.get(`${apiBase}/voice/rooms/:id/stream`, ({ request }) =>
      withViewer(request, () => {
        sessions.push(new URL(request.url).searchParams.get("session") ?? "");
        const body = new ReadableStream<Uint8Array>({
          start(controller) {
            stream = controller;
            controller.enqueue(encoder.encode(": connected\n\n"));
          },
        });
        return new HttpResponse(body, { headers: { "Content-Type": "text/event-stream" } });
      }),
    ),
    ...(["signals", "mic", "reactions"] as const).map((op) =>
      http.post(`${apiBase}/voice/rooms/:id/${op}`, async ({ request }) => {
        await record(request);
        return new HttpResponse(null, { status: 204 });
      }),
    ),
    http.post(`${apiBase}/voice/rooms/:id/speaker`, async ({ request }) => {
      await record(request);
      return respondRoom({ ...current, participants: current.participants.map((p) => (p.account.id === viewerFixture.id ? { ...p, role: "speaker", hand_raised: false } : p)) });
    }),
    http.post(`${apiBase}/voice/rooms/:id/hand`, async ({ request }) => {
      await record(request);
      return respondRoom({ ...current, participants: current.participants.map((p) => (p.account.id === viewerFixture.id ? { ...p, hand_raised: true } : p)) });
    }),
    http.post(`${apiBase}/voice/rooms/:id/participants/:accountId/grant`, async ({ request, params }) => {
      await record(request);
      return respondRoom({ ...current, participants: current.participants.map((p) => (p.account.id === params["accountId"] ? { ...p, role: "speaker", hand_raised: false } : p)) });
    }),
  );
  return {
    sessions,
    requests,
    /** Sends an event on the room stream. */
    send: (event: string, data: unknown) => {
      stream?.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
    },
    sent: (path: string) => requests.filter((r) => r.path === path).map((r) => r.body),
    /** Ends the room: the server no longer shows it, and its summary says it ended. */
    end: () => {
      ended = true;
    },
  };
}

const getUserMedia = vi.fn(() => Promise.resolve(new FakeStream() as unknown as MediaStream));

beforeEach(() => {
  resetFakeRtc();
  getUserMedia.mockClear();
  vi.stubGlobal("RTCPeerConnection", FakePeerConnection);
  vi.stubGlobal("AudioContext", FakeAudioContext);
  Object.defineProperty(navigator, "mediaDevices", { value: { getUserMedia }, configurable: true });
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue(undefined);
  vi.spyOn(HTMLMediaElement.prototype, "pause").mockReturnValue(undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator, "mediaDevices");
});

/** Bob hosts and speaks with the session s9; Alice, the user, joins as a listener with s1. */
function bobsRoom() {
  const room = voiceRoomFixture();
  const joined = voiceRoomFixture({ participants: [participantFixture(otherAccountFixture, { role: "speaker", host: true }), participantFixture(viewerFixture)] });
  const live = voiceRoomFixture({
    participants: [participantFixture(otherAccountFixture, { role: "speaker", host: true, session: "s9" }), participantFixture(viewerFixture, { session: "s1" })],
  });
  return { room, joined, live };
}

describe("a listener in the call", () => {
  test("joins, sends heartbeats, answers the host's offer receiving only, and sees the host's microphone and reactions", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    expect(await screen.findByText("When you join, your name is listed for everyone who can see this room.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Join" }));
    expect(srv.requests.filter((r) => r.path === `/voice/rooms/${roomId}/participation`)).toHaveLength(1);

    expect(await screen.findByText("Connected")).toBeInTheDocument();
    expect(srv.sessions).toEqual(["s1"]);
    await waitFor(() => {
      expect(srv.sent(`/voice/rooms/${roomId}/heartbeat`)).toEqual([{ session: "s1" }]);
    });

    srv.send("state", live);
    srv.send("signal", { from: "s9", kind: "offer", sdp: "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=sendrecv\r\n" });
    await waitFor(() => {
      expect(srv.sent(`/voice/rooms/${roomId}/signals`)).toContainEqual(expect.objectContaining({ session: "s1", to: "s9", kind: "answer" }));
    });
    const answer = srv.sent(`/voice/rooms/${roomId}/signals`).find((s) => (s as Schemas["VoiceSignal"]).kind === "answer") as Schemas["VoiceSignal"];
    expect(sdpDirections(answer.sdp ?? "")).toEqual(["recvonly"]);
    expect(FakePeerConnection.all).toHaveLength(1);

    srv.send("mic", { participant: otherAccountFixture.id, on: true });
    srv.send("reaction", { participant: otherAccountFixture.id, emoji: "🎉" });
    const participants = screen.getByRole("region", { name: "Participants" });
    const bob = within(participants).getAllByRole("listitem")[0] ?? participants;
    expect(await within(bob).findByText("Mic on")).toBeInTheDocument();
    expect(await within(bob).findByText("🎉")).toBeInTheDocument();
    expect(within(participants).getByText("You")).toBeInTheDocument();
    // A listener has no microphone.
    expect(screen.queryByRole("switch", { name: "Microphone" })).not.toBeInTheDocument();
  });

  test("raises a hand, becomes a speaker, turns the microphone on, and a host's mute turns it off", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);

    await user.click(screen.getByRole("button", { name: "Raise hand" }));
    expect(await screen.findByRole("button", { name: "Lower hand" })).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Participants" })).getByText("Hand raised")).toBeInTheDocument();

    srv.send("state", { ...live, participants: live.participants.map((p) => (p.account.id === viewerFixture.id ? { ...p, role: "speaker" } : p)) });
    expect(await screen.findByText("You can speak now. Turn on the microphone to talk.")).toBeInTheDocument();
    const mic = await screen.findByRole("switch", { name: "Microphone" });
    expect(mic).not.toBeChecked();
    await user.click(mic);
    await waitFor(() => {
      expect(srv.sent(`/voice/rooms/${roomId}/mic`)).toEqual([{ session: "s1", on: true }]);
    });
    expect(getUserMedia).toHaveBeenCalledWith({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }, video: false });
    expect(mic).toBeChecked();

    srv.send("muted", { by: otherAccountFixture.id });
    expect(await screen.findByText("A host turned off your microphone. You can turn it on again.")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("switch", { name: "Microphone" })).not.toBeChecked();
    });
    // The server turned it off for everyone: the app does not tell it again.
    expect(srv.sent(`/voice/rooms/${roomId}/mic`)).toEqual([{ session: "s1", on: true }]);
  });

  test("turns the microphone on right after speaking, before the next state", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);
    await user.click(await screen.findByRole("button", { name: "Speak" }));
    await user.click(await screen.findByRole("switch", { name: "Microphone" }));
    await waitFor(() => {
      expect(srv.sent(`/voice/rooms/${roomId}/mic`)).toEqual([{ session: "s1", on: true }]);
    });
    expect(getUserMedia).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("switch", { name: "Microphone" })).toBeChecked();
  });

  test("says why the browser did not give the microphone", async () => {
    getUserMedia.mockRejectedValueOnce(new DOMException("no", "NotFoundError"));
    const { room, joined } = bobsRoom();
    serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    await user.click(await screen.findByRole("button", { name: "Speak" }));
    await user.click(await screen.findByRole("switch", { name: "Microphone" }));
    expect(await screen.findByText("No microphone was found. Connect one, then try again.")).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Microphone" })).not.toBeChecked();
  });

  test("sends a reaction with the picker", async () => {
    const { room, joined } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    await user.click(screen.getByRole("button", { name: "React" }));
    const dialog = await screen.findByRole("dialog");
    const cells = await within(dialog).findAllByRole("gridcell");
    await user.click(cells[0] ?? dialog);
    await waitFor(() => {
      expect(srv.sent(`/voice/rooms/${roomId}/reactions`)).toHaveLength(1);
    });
  });

  test("leaves the room, and the end of the room stays on the page", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);
    await user.click(screen.getByRole("button", { name: "Leave" }));
    expect(await screen.findByRole("button", { name: "Join" })).toBeInTheDocument();
    expect(srv.requests.some((r) => r.method === "DELETE" && r.path === `/voice/rooms/${roomId}/participation`)).toBe(true);

    await user.click(screen.getByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("ended", { reason: "host" });
    expect(await screen.findAllByText("The room ended: The host ended it")).not.toHaveLength(0);
  });

  test("the page of a room that ended while it was open says so, without the way to join", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);
    srv.end();
    srv.send("ended", { reason: "host" });
    expect(await screen.findByRole("heading", { level: 1, name: "This room has ended" })).toBeInTheDocument();
    expect(screen.getByText(room.name)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Join" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Participants" })).not.toBeInTheDocument();
  });
});

describe("the call across pages", () => {
  test("goes on with the call bar on other pages, which leads back to the room and leaves it", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);
    // The room's page has the call's controls itself: no bar there.
    expect(screen.queryByRole("region", { name: "Voice chat call" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("link", { name: "Home" }));
    const bar = await screen.findByRole("region", { name: "Voice chat call" });
    expect(within(bar).getByText("Connected")).toBeInTheDocument();
    // The call goes on: the same session and stream.
    expect(srv.sessions).toEqual(["s1"]);
    expect(FakePeerConnection.all.every((pc) => !pc.closed)).toBe(true);

    await user.click(within(bar).getByRole("link", { name: "Friday night radio" }));
    expect(await screen.findByRole("heading", { level: 1, name: "Friday night radio" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Voice chat call" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Raise hand" })).toBeInTheDocument();

    await user.click(screen.getByRole("link", { name: "Home" }));
    const again = await screen.findByRole("region", { name: "Voice chat call" });
    await user.click(within(again).getByRole("button", { name: "Leave" }));
    await waitFor(() => {
      expect(screen.queryByRole("region", { name: "Voice chat call" })).not.toBeInTheDocument();
    });
    expect(srv.requests.some((r) => r.method === "DELETE" && r.path === `/voice/rooms/${roomId}/participation`)).toBe(true);
  });
});

describe("the end of the call with the app", () => {
  test("signing out leaves the room before the token is revoked", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    server.use(
      http.post(oauthPaths.revoke, () => {
        srv.requests.push({ method: "POST", path: "revoke", body: undefined });
        return new HttpResponse(null, { status: 200 });
      }),
    );
    const { user, session } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);
    await act(() => session.signOut());
    const order = srv.requests.map((r) => `${r.method} ${r.path}`);
    const left = order.indexOf(`DELETE /voice/rooms/${roomId}/participation`);
    expect(left).toBeGreaterThanOrEqual(0);
    expect(left).toBeLessThan(order.indexOf("POST revoke"));
    expect(FakePeerConnection.all.every((pc) => pc.closed)).toBe(true);
  });

  test("the maintenance screen, which replaces the pages, ends the call", async () => {
    const { room, joined, live } = bobsRoom();
    const srv = serve(room, joined);
    const { user, api } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");
    srv.send("state", live);
    let maintenance = false;
    server.use(
      http.all("*", async ({ request }) => {
        if (!maintenance || !new URL(request.url).pathname.startsWith("/api/")) return undefined;
        if (request.method === "DELETE") srv.requests.push({ method: "DELETE", path: new URL(request.url).pathname.slice(apiBase.length), body: undefined });
        await Promise.resolve();
        return HttpResponse.json({ type: "/api/v1/problems/maintenance", title: "Under maintenance", status: 503 }, { status: 503, headers: { "Content-Type": "application/problem+json", "Retry-After": "600" } });
      }),
    );
    maintenance = true;
    await act(() => api.fetch.GET("/instance").catch(() => undefined));
    expect(await screen.findByRole("heading", { level: 1, name: "Under maintenance" })).toBeInTheDocument();
    await waitFor(() => {
      expect(srv.requests.some((r) => r.method === "DELETE" && r.path === `/voice/rooms/${roomId}/participation`)).toBe(true);
    });
    expect(FakePeerConnection.all.every((pc) => pc.closed)).toBe(true);
  });
});

describe("the host in the call", () => {
  function hostRoom() {
    const bob = participantFixture(otherAccountFixture, { hand_raised: true });
    const room = voiceRoomFixture({ host: viewerFixture, participants: [participantFixture(viewerFixture, { role: "speaker", host: true }), bob] });
    return room;
  }

  test("is in the call of the room they start, without joining it again", async () => {
    const room = voiceRoomFixture({ host: viewerFixture, participants: [participantFixture(viewerFixture, { role: "speaker", host: true })] });
    const srv = serve(room, room);
    server.use(
      http.get(`${apiBase}/voice/rooms`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
      http.post(`${apiBase}/voice/rooms`, ({ request }) => withViewer(request, () => HttpResponse.json({ room, session: "s1", grant }, { status: 201 }))),
    );
    const { user } = await renderApp({ path: "/voice" });
    await user.click(await screen.findByRole("button", { name: "Start a room" }));
    const dialog = await screen.findByRole("dialog", { name: "Start a room" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Late show");
    await user.click(within(dialog).getByRole("button", { name: "Start the room" }));

    expect(await screen.findByText("Connected")).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: "Microphone" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Join here" })).not.toBeInTheDocument();
    expect(srv.sessions).toEqual(["s1"]);
    expect(srv.requests.filter((r) => r.path === `/voice/rooms/${roomId}/participation`)).toHaveLength(0);
  });

  test("lets a participant with a raised hand speak from the participant's menu", async () => {
    const room = hostRoom();
    const srv = serve(room, room);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");

    await user.click(screen.getByRole("button", { name: "Actions for Bob" }));
    const menu = await screen.findByRole("menu", { name: "Actions for Bob" });
    expect(within(menu).getAllByRole("menuitem").map((item) => item.textContent)).toEqual(["Let speak", "Dismiss raised hand", "Remove from room", "Make co-host", "Make host"]);
    await user.click(within(menu).getByRole("menuitem", { name: "Let speak" }));
    expect(await screen.findByText("Bob can speak now.")).toBeInTheDocument();
    expect(srv.requests.some((r) => r.path === `/voice/rooms/${roomId}/participants/${otherAccountFixture.id}/grant`)).toBe(true);
  });

  test("chooses the next host when leaving a room with others in it", async () => {
    const room = hostRoom();
    const srv = serve(room, room);
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Join" }));
    await screen.findByText("Connected");

    await user.click(screen.getByRole("button", { name: "Leave" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Leave the room you host?" });
    await user.click(within(dialog).getByRole("radio", { name: "Make Bob the host" }));
    await user.click(within(dialog).getByRole("button", { name: "Leave" }));
    await waitFor(() => {
      expect(srv.requests.some((r) => r.method === "DELETE" && r.path === `/voice/rooms/${roomId}/participation?next_host_id=${otherAccountFixture.id}`)).toBe(true);
    });
  });
});
