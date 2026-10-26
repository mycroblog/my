// SPDX-License-Identifier: AGPL-3.0-only
// Voice chat through the whole app before the call itself (web-client/README.md T4, ADR-0100 §8, §9, §13): MSW plays
// the server. Nothing shows while the server has voice chat off; with it on, the navigation leads to the list of the
// open rooms, where the user starts a room with its start post; the room's page lists its participants and says that
// joining lists the user's name; the host changes the settings; visitors who are not signed in get the summary; the
// hashtag's page, the profile, and the settings' history show the rooms.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, otherAccountFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, problem, signedInHandler, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { ownRoomFixture, participantFixture, roomId, voiceRoomFixture } from "./test/handlers";

const carol: Schemas["Account"] = { ...otherAccountFixture, id: "0192f0c4-7a8e-7000-8000-000000000033", username: "carol", acct: "carol", display_name: "Carol" };

/**
 * The server with voice chat on, where `starters` open rooms, and `rooms` open, answering the list (also by hashtag)
 * and each room.
 */
function voiceOn(rooms: Schemas["VoiceRoom"][] = [voiceRoomFixture()], starters: Schemas["Instance"]["voice_room_starters"] = "everyone") {
  server.use(
    http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, voice_enabled: true, voice_room_starters: starters })),
    http.get(`${apiBase}/voice/rooms`, ({ request }) =>
      withViewer(request, () => {
        const tag = new URL(request.url).searchParams.get("tag");
        return HttpResponse.json({ items: tag === null ? rooms : rooms.filter((room) => room.hashtags.includes(tag)) });
      }),
    ),
    http.get(`${apiBase}/voice/rooms/:id`, ({ request, params }) =>
      withViewer(request, () => {
        const room = rooms.find((r) => r.id === params["id"]);
        return room === undefined ? HttpResponse.json({ type: "about:blank", title: "Not Found", status: 404 }, { status: 404 }) : HttpResponse.json(room);
      }),
    ),
  );
}

describe("while the server has voice chat off", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("the navigation and the settings menu have no entry to it", async () => {
    await renderApp({ path: "/settings" });
    const menu = await screen.findByRole("navigation", { name: "Settings pages" });
    expect(within(menu).getByRole("link", { name: "Profile" })).toBeInTheDocument();
    expect(within(menu).queryByRole("link", { name: "Voice chat history" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Voice chat" })).not.toBeInTheDocument();
  });

  test("the list page says that the server does not offer it", async () => {
    await renderApp({ path: "/voice" });
    expect(await screen.findByRole("heading", { name: "This server does not offer voice chat." })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Start a room" })).not.toBeInTheDocument();
  });

  test("a room's page says there is no such room", async () => {
    await renderApp({ path: `/voice/${roomId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "There is no such room" })).toBeInTheDocument();
  });
});

describe("the list of the open rooms", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("opens from the navigation and shows each room with its host, count, description, and hashtags", async () => {
    voiceOn([voiceRoomFixture(), voiceRoomFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000f2", name: "Quiet study", description: "", hashtags: [], participants_count: 1 })]);
    const { user } = await renderApp({ path: "/home" });
    await user.click(await screen.findByRole("link", { name: "Voice chat" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Voice chat" })).toBeInTheDocument();
    const list = screen.getByRole("list", { name: "Open rooms" });
    const rooms = within(list).getAllByRole("heading", { level: 2 });
    expect(rooms.map((heading) => heading.textContent)).toEqual(["Friday night radio", "Quiet study"]);
    expect(within(list).getByRole("link", { name: "Friday night radio" })).toHaveAttribute("href", `/voice/${roomId}`);
    expect(within(list).getAllByRole("link", { name: /Bob/u })[0]).toHaveAttribute("href", "/@bob");
    expect(within(list).getByText("Talking about the week's music.")).toHaveAttribute("dir", "auto");
    expect(within(list).getByText("#music")).toBeInTheDocument();
    expect(within(list).getAllByText("1 person")).toHaveLength(2);
  });

  test("says so when no room is open", async () => {
    voiceOn([]);
    await renderApp({ path: "/voice" });
    expect(await screen.findByRole("heading", { name: "No rooms are open right now" })).toBeInTheDocument();
  });

  test("starts a room with the start post drafted from its name, then opens the room's page", async () => {
    voiceOn([]);
    const bodies: unknown[] = [];
    server.use(
      http.post(`${apiBase}/voice/rooms`, async ({ request }) => {
        const body = (await request.json()) as Schemas["VoiceRoomSettings"];
        bodies.push(body);
        const room = ownRoomFixture({ name: body.name, hashtags: body.hashtags ?? [], visibility: body.visibility ?? "local" });
        voiceOn([room]);
        return withViewer(request, () => HttpResponse.json({ room, session: "s1", grant: { kind: "p2p", ice_servers: [], relay_only: false, expires_at: "2026-10-26T18:00:00Z" } }, { status: 201 }));
      }),
    );
    const { user } = await renderApp({ path: "/voice" });
    await user.click(await screen.findByRole("button", { name: "Start a room" }));
    const dialog = await screen.findByRole("dialog", { name: "Start a room" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Late show");
    await user.type(within(dialog).getByRole("textbox", { name: /Hashtags/u }), "#music talk");
    await user.click(within(within(dialog).getByRole("radiogroup", { name: "Who can see and join" })).getByRole("radio", { name: "Followers of the host" }));
    expect(within(dialog).getByRole("switch", { name: "Post about the room" })).toBeChecked();
    expect(within(dialog).getByRole("switch", { name: "Post about the room" })).toHaveAccessibleDescription("A post for your followers only.");
    const text = within(dialog).getByRole("textbox", { name: "Post text" });
    expect(text).toHaveValue("I opened a voice chat room: Late show");
    await user.type(text, " — come by");
    await user.click(within(dialog).getByRole("button", { name: "Start the room" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Late show" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(bodies).toEqual([
      {
        name: "Late show",
        description: "",
        hashtags: ["music", "talk"],
        visibility: "followers",
        speakers: "manual",
        post: { text: "I opened a voice chat room: Late show — come by" },
      },
    ]);
  });

  test("offers no start post for a room of mutual follows, and checks the name before sending", async () => {
    voiceOn([]);
    const { user } = await renderApp({ path: "/voice" });
    await user.click(await screen.findByRole("button", { name: "Start a room" }));
    const dialog = await screen.findByRole("dialog", { name: "Start a room" });
    await user.click(within(within(dialog).getByRole("radiogroup", { name: "Who can see and join" })).getByRole("radio", { name: "Mutual follows of the host" }));
    expect(within(dialog).queryByRole("switch", { name: "Post about the room" })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Start the room" }));
    expect(await within(dialog).findByText("Enter a name of up to 100 characters on one line.")).toBeInTheDocument();
  });
});

describe("starting a room", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("is offered only to a user who may open rooms", async () => {
    voiceOn([], "permitted");
    const { unmount } = await renderApp({ path: "/voice" });
    expect(await screen.findByRole("heading", { name: "No rooms are open right now" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Start a room" })).not.toBeInTheDocument();
    unmount();

    server.use(operatorHandler(["start_voice_rooms"]));
    await renderApp({ path: "/voice" });
    expect(await screen.findByRole("button", { name: "Start a room" })).toBeInTheDocument();
  });

  test("says why the server refused the room, and keeps the form", async () => {
    voiceOn([]);
    server.use(http.post(`${apiBase}/voice/rooms`, () => problem(403, "/api/v1/problems/voice-not-eligible", "You may not start voice chat rooms")));
    const { user } = await renderApp({ path: "/voice" });
    await user.click(await screen.findByRole("button", { name: "Start a room" }));
    const dialog = await screen.findByRole("dialog", { name: "Start a room" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Late show");
    await user.click(within(dialog).getByRole("button", { name: "Start the room" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Couldn't start the room: You may not start voice chat rooms");
    expect(within(dialog).getByRole("textbox", { name: "Name" })).toHaveValue("Late show");
  });

  test("checks the hashtags before sending, and moves focus to the field", async () => {
    voiceOn([]);
    let sent = false;
    server.use(
      http.post(`${apiBase}/voice/rooms`, () => {
        sent = true;
        return problem(500, "about:blank", "Unexpected");
      }),
    );
    const { user } = await renderApp({ path: "/voice" });
    await user.click(await screen.findByRole("button", { name: "Start a room" }));
    const dialog = await screen.findByRole("dialog", { name: "Start a room" });
    await user.type(within(dialog).getByRole("textbox", { name: "Name" }), "Late show");
    const hashtags = within(dialog).getByRole("textbox", { name: /Hashtags/u });
    // The same hashtag in another case counts once, so these are six.
    await user.type(hashtags, "a b c d e Music music");
    await user.click(within(dialog).getByRole("button", { name: "Start the room" }));
    expect(await within(dialog).findByText("Enter up to 5 hashtags.")).toBeInTheDocument();
    expect(hashtags).toHaveFocus();

    await user.clear(hashtags);
    await user.type(hashtags, "x".repeat(101));
    await user.click(within(dialog).getByRole("button", { name: "Start the room" }));
    expect(await within(dialog).findByText("Each hashtag can have up to 100 characters.")).toBeInTheDocument();
    expect(sent).toBe(false);
  });
});

describe("the page of a room", () => {
  test("lists everyone with their role and says that joining lists the user's name", async () => {
    server.use(signedInHandler);
    voiceOn([voiceRoomFixture({ participants: [participantFixture(otherAccountFixture, { role: "speaker", host: true }), participantFixture(carol, { cohost: true })] })]);
    let joined = false;
    server.use(
      http.post(`${apiBase}/voice/rooms/:id/participation`, ({ request }) => {
        joined = true;
        const room = voiceRoomFixture({ participants: [participantFixture(otherAccountFixture, { role: "speaker", host: true }), participantFixture(viewerFixture)] });
        return withViewer(request, () => HttpResponse.json({ room, session: "s1", grant: { kind: "p2p", ice_servers: [], relay_only: false, expires_at: "2026-10-26T18:00:00Z" } }));
      }),
    );
    const { user } = await renderApp({ path: `/voice/${roomId}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Friday night radio" })).toBeInTheDocument();
    const participants = screen.getByRole("region", { name: "Participants" });
    const items = within(participants).getAllByRole("listitem");
    expect(items.map((item) => item.textContent)).toEqual([expect.stringMatching(/Bob.*Host/u), expect.stringMatching(/Carol.*Co-host/u)]);
    expect(screen.getByText("When you join, your name is listed for everyone who can see this room.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Room settings" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Join" }));
    expect(await screen.findByText("You are in this room.")).toBeInTheDocument();
    expect(joined).toBe(true);
    expect(within(screen.getByRole("region", { name: "Participants" })).getByText(/Alice/u)).toBeInTheDocument();
  });

  test("lets the host change the settings, sending only what changed", async () => {
    server.use(signedInHandler);
    voiceOn([ownRoomFixture()]);
    const updates: unknown[] = [];
    server.use(
      http.patch(`${apiBase}/voice/rooms/:id`, async ({ request }) => {
        const update = (await request.json()) as Schemas["VoiceRoomUpdate"];
        updates.push(update);
        const room = ownRoomFixture({ ...update });
        voiceOn([room]);
        return withViewer(request, () => HttpResponse.json(room));
      }),
    );
    const { user } = await renderApp({ path: `/voice/${roomId}` });
    await user.click(await screen.findByRole("button", { name: "Room settings" }));
    const dialog = await screen.findByRole("dialog", { name: "Room settings" });
    expect(within(dialog).getByRole("textbox", { name: "Name" })).toHaveValue("Friday night radio");
    expect(within(dialog).queryByRole("switch", { name: "Post about the room" })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("radio", { name: "Everyone who joins" }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(updates).toEqual([{ speakers: "everyone" }]);
    expect(await screen.findByText("Everyone who joins")).toBeInTheDocument();
  });

  test("tells a visitor who is not signed in that only this server's users join, with the summary the server shows", async () => {
    server.use(
      http.get(`${apiBase}/voice/rooms/:id/summary`, () =>
        HttpResponse.json({ id: roomId, state: "open", name: "Friday night radio", description: "Talking about music.", host: otherAccountFixture }),
      ),
    );
    await renderApp({ path: `/voice/${roomId}` });

    expect(await screen.findByRole("heading", { level: 1, name: "Friday night radio" })).toBeInTheDocument();
    expect(screen.getByText("Only people with an account on this server can join its voice chat rooms.")).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("link", { name: "Sign in" })).toHaveAttribute("href", `/login?return_to=%2Fvoice%2F${roomId}`);
    expect(screen.queryByRole("button", { name: "Join" })).not.toBeInTheDocument();
  });

  test("names no room to a visitor when the server shows no summaries", async () => {
    server.use(http.get(`${apiBase}/voice/rooms/:id/summary`, () => HttpResponse.json({ id: roomId, state: "open" })));
    await renderApp({ path: `/voice/${roomId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Voice chat room" })).toBeInTheDocument();
    expect(within(screen.getByRole("main")).getByRole("link", { name: "Sign in" })).toBeInTheDocument();
  });

  test("tells a signed-in user who does not see an open room so, without its participants", async () => {
    server.use(signedInHandler);
    voiceOn([]);
    server.use(http.get(`${apiBase}/voice/rooms/:id/summary`, () => HttpResponse.json({ id: roomId, state: "open" })));
    await renderApp({ path: `/voice/${roomId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "Voice chat room" })).toBeInTheDocument();
    expect(screen.getByText("You can't see this room.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Join" })).not.toBeInTheDocument();
    expect(within(screen.getByRole("main")).queryByRole("link", { name: "Sign in" })).not.toBeInTheDocument();
  });

  test("says that a room ended, with its name from the summary", async () => {
    server.use(signedInHandler);
    voiceOn([]);
    server.use(http.get(`${apiBase}/voice/rooms/:id/summary`, () => HttpResponse.json({ id: roomId, state: "ended", name: "Friday night radio" })));
    await renderApp({ path: `/voice/${roomId}` });
    expect(await screen.findByRole("heading", { level: 1, name: "This room has ended" })).toBeInTheDocument();
    expect(screen.getByText("Friday night radio")).toBeInTheDocument();
  });
});

describe("the rooms elsewhere", () => {
  beforeEach(() => {
    server.use(signedInHandler);
  });

  test("the hashtag's page shows its open rooms above its posts", async () => {
    voiceOn([voiceRoomFixture(), voiceRoomFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000f2", name: "Cat talk", hashtags: ["cats"] })]);
    await renderApp({ path: "/tags/cats" });
    const section = await screen.findByRole("region", { name: "Voice chat rooms with #cats" });
    expect(within(section).getAllByRole("heading", { level: 3 }).map((heading) => heading.textContent)).toEqual(["Cat talk"]);
  });

  test("a profile links the room where the account hosts", async () => {
    voiceOn();
    server.use(
      http.get(`${apiBase}/accounts/:id`, ({ request, params }) => {
        if (params["id"] !== otherAccountFixture.id) return undefined;
        return withViewer(request, () => HttpResponse.json({ ...otherAccountFixture, voice_room: { id: roomId, name: "Friday night radio", role: "host", participants_count: 3 } }));
      }),
    );
    await renderApp({ path: "/@bob" });
    const link = await screen.findByRole("link", { name: "Friday night radio" });
    expect(link).toHaveAttribute("href", `/voice/${roomId}`);
    expect(screen.getByText(/is hosting a voice chat room/u)).toHaveTextContent("⁨Bob⁩ is hosting a voice chat room: Friday night radio");
    expect(screen.getByText("3 people")).toBeInTheDocument();
  });

  test("the settings list the rooms the user hosted", async () => {
    voiceOn();
    server.use(
      http.get(`${apiBase}/voice/history`, ({ request }) =>
        withViewer(request, () =>
          HttpResponse.json({
            items: [
              { id: "r2", name: "Late show", opened_at: "2026-10-26T20:00:00Z" },
              { id: "r1", name: "Morning call", opened_at: "2026-10-25T08:00:00Z", ended_at: "2026-10-25T09:00:00Z", end_reason: "empty" },
            ],
          }),
        ),
      ),
    );
    const { user } = await renderApp({ path: "/settings" });
    await user.click(await screen.findByRole("link", { name: "Voice chat history" }));
    expect(await screen.findByRole("heading", { level: 2, name: "Late show" })).toBeInTheDocument();
    expect(screen.getByText("Still open")).toBeInTheDocument();
    expect(screen.getByText("Everyone left")).toBeInTheDocument();
  });
});
