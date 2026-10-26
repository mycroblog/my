// SPDX-License-Identifier: MIT
import { describe, expect, test } from "bun:test";
import { createClient } from "./client.js";
import { fakeFetch, json, server } from "./test/fake.js";
import { staticToken } from "./token.js";

// Every voice operation of the REST API goes through the typed client. The stream and the signals belong to the WebRTC
// connection, which mykit does not have.
function setup() {
  const f = fakeFetch(() => new Response(null, { status: 204 }));
  const api = createClient({ server, fetch: f.fetch, tokenSource: staticToken("t1") });
  const last = () => {
    const sent = f.requests.at(-1);
    return { method: sent?.request.method, url: sent?.request.url.slice(`${server}/api/v1`.length), body: sent?.body, auth: sent?.request.headers.get("Authorization") };
  };
  return { api, last };
}

const id = "r1";
const accountId = "a1";
const room = { params: { path: { id } } };
const member = { params: { path: { id, account_id: accountId } } };

describe("voice rooms", () => {
  test("list, open, get, update, end, summary, and history", async () => {
    const { api, last } = setup();
    await api.GET("/voice/rooms", { params: { query: { tag: "go", cursor: "c1" } } });
    expect(last()).toEqual({ method: "GET", url: "/voice/rooms?tag=go&cursor=c1", body: "", auth: "Bearer t1" });
    await api.POST("/voice/rooms", { body: { name: "Late night" } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms", body: '{"name":"Late night"}' });
    await api.GET("/voice/rooms/{id}", room);
    expect(last()).toMatchObject({ method: "GET", url: "/voice/rooms/r1" });
    await api.PATCH("/voice/rooms/{id}", { ...room, body: { name: "Later" } });
    expect(last()).toMatchObject({ method: "PATCH", url: "/voice/rooms/r1", body: '{"name":"Later"}' });
    await api.DELETE("/voice/rooms/{id}", room);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1" });
    await api.GET("/voice/rooms/{id}/summary", room);
    expect(last()).toMatchObject({ method: "GET", url: "/voice/rooms/r1/summary" });
    await api.GET("/voice/history", {});
    expect(last()).toMatchObject({ method: "GET", url: "/voice/history" });
  });

  test("participation, heartbeat, hand-over, speaking, hand, mic, grant, and reactions", async () => {
    const { api, last } = setup();
    await api.POST("/voice/rooms/{id}/participation", room);
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/participation" });
    await api.DELETE("/voice/rooms/{id}/participation", { params: { path: { id }, query: { next_host_id: accountId, end: true } } });
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/participation?next_host_id=a1&end=true" });
    await api.POST("/voice/rooms/{id}/heartbeat", { ...room, body: { session: "s1" } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/heartbeat", body: '{"session":"s1"}' });
    await api.POST("/voice/rooms/{id}/host", { ...room, body: { account_id: accountId } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/host", body: '{"account_id":"a1"}' });
    await api.POST("/voice/rooms/{id}/speaker", room);
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/speaker" });
    await api.DELETE("/voice/rooms/{id}/speaker", room);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/speaker" });
    await api.POST("/voice/rooms/{id}/hand", room);
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/hand" });
    await api.DELETE("/voice/rooms/{id}/hand", room);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/hand" });
    await api.POST("/voice/rooms/{id}/mic", { ...room, body: { on: true, session: "s1" } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/mic", body: '{"on":true,"session":"s1"}' });
    await api.POST("/voice/rooms/{id}/grant", { ...room, body: { session: "s1" } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/grant", body: '{"session":"s1"}' });
    await api.POST("/voice/rooms/{id}/reactions", { ...room, body: { emoji: "🎉" } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/reactions", body: '{"emoji":"🎉"}' });
  });

  test("participant operations and invitations", async () => {
    const { api, last } = setup();
    await api.POST("/voice/rooms/{id}/participants/{account_id}/grant", member);
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/participants/a1/grant" });
    await api.DELETE("/voice/rooms/{id}/participants/{account_id}/grant", member);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/participants/a1/grant" });
    await api.DELETE("/voice/rooms/{id}/participants/{account_id}/hand", member);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/participants/a1/hand" });
    await api.POST("/voice/rooms/{id}/participants/{account_id}/mute", member);
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/participants/a1/mute" });
    await api.DELETE("/voice/rooms/{id}/participants/{account_id}", member);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/participants/a1" });
    await api.POST("/voice/rooms/{id}/participants/{account_id}/cohost", member);
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/participants/a1/cohost" });
    await api.DELETE("/voice/rooms/{id}/participants/{account_id}/cohost", member);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/participants/a1/cohost" });
    await api.GET("/voice/rooms/{id}/invitations", room);
    expect(last()).toMatchObject({ method: "GET", url: "/voice/rooms/r1/invitations" });
    await api.POST("/voice/rooms/{id}/invitations", { ...room, body: { account_id: accountId } });
    expect(last()).toMatchObject({ method: "POST", url: "/voice/rooms/r1/invitations", body: '{"account_id":"a1"}' });
    await api.DELETE("/voice/rooms/{id}/invitations/{account_id}", member);
    expect(last()).toMatchObject({ method: "DELETE", url: "/voice/rooms/r1/invitations/a1" });
  });

  test("a moderator ends a room, and an error answer is a problem", async () => {
    const { api, last } = setup();
    await api.POST("/admin/voice_rooms/{id}/end", { ...room, body: {} });
    expect(last()).toMatchObject({ method: "POST", url: "/admin/voice_rooms/r1/end", body: "{}" });
    const f = fakeFetch(() => json(404, { type: "about:blank", title: "Not Found", status: 404 }));
    const failing = createClient({ server, fetch: f.fetch });
    const error: unknown = await failing.GET("/voice/rooms/{id}", room).catch((e: unknown) => e);
    expect((error as { status?: number }).status).toBe(404);
  });
});
