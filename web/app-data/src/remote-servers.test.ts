// SPDX-License-Identifier: AGPL-3.0-only

// The remote servers as data (#3478): the list asks the server for every known server, for those with problems
// (`troubled=true`) or for those of a state, and carries each server's software and when it was first seen as the
// server reports them.

import { describe, expect, test } from "bun:test";
import { loadRemoteServers, remoteServerQuery, type RemoteServerQuery } from "./remote-servers";
import { fakeData } from "./test/data";

/** The query of each read of the list the app's data sent. */
const listReads = (requests: readonly string[]) =>
  requests
    .filter((r) => r.startsWith("GET /admin/instances") && !r.startsWith("GET /admin/instances/"))
    .map((r) => new URL(r.slice("GET ".length), "https://demo.test").searchParams);

/** What the list read with `q` asked the server, by marco, who manages domains. */
async function listed(q: RemoteServerQuery) {
  const { data, requests } = fakeData({ viewer: "marco" });
  await loadRemoteServers(data, q);
  const reads = listReads(requests);
  expect(reads).toHaveLength(1);
  const [asked] = reads;
  if (asked === undefined) throw new Error("test: the list was not read");
  return { asked };
}

describe("loadRemoteServers", () => {
  test("asks for every known server with no filter", async () => {
    const { asked } = await listed({});
    expect(asked.get("troubled")).toBeNull();
    expect(asked.get("state")).toBeNull();
  });

  test("asks for the servers with problems with troubled=true", async () => {
    const { asked } = await listed({ troubled: true });
    expect(asked.get("troubled")).toBe("true");
    expect(asked.get("state")).toBeNull();
  });

  for (const state of ["available", "unavailable"] as const)
    test(`asks for the servers ${state} with state=${state}`, async () => {
      const { asked } = await listed({ state });
      expect(asked.get("state")).toBe(state);
      expect(asked.get("troubled")).toBeNull();
    });
});

describe("remoteServerQuery", () => {
  test("carries a server's software and version and when it was first seen, as the server reports them", async () => {
    const { data } = fakeData({ viewer: "marco" });
    const server = await data.queryClient.query(remoteServerQuery(data, "photos.example"));
    expect(server.software).toEqual({ name: "pixelfed", version: "0.12.4" });
    expect(typeof server.first_seen_at).toBe("string");
  });

  test("carries no software and no first sighting for a server that reports none", async () => {
    const { data } = fakeData({ viewer: "marco" });
    const server = await data.queryClient.query(remoteServerQuery(data, "quiet.example"));
    expect(server.software).toBeUndefined();
    expect(server.first_seen_at).toBeUndefined();
  });
});
