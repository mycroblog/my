// SPDX-License-Identifier: AGPL-3.0-only

// The suggested accounts (FR-308): read from the server in its order, the accounts alone; and nothing asked while a
// screen does not want them.

import { describe, expect, test } from "bun:test";
import { suggestionsQuery, useSuggestions } from "./suggestions";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";

// In the scenario the server suggests to the reader mio, who posted last, then marco: the accounts of this server
// they neither follow, block nor mute.
describe("suggestions", () => {
  test("the query reads the suggestions, and the hook shows their accounts in the server's order", async () => {
    const { fake, data, requests } = fakeData();
    await data.queryClient.query(suggestionsQuery(data));
    expect(requests[0]).toBe("GET /accounts/suggestions");
    const shown = probe(data, () => useSuggestions());
    expect(shown.status).toBe("success");
    expect(shown.items.map((a) => a.id)).toEqual(["mio", "marco"].map(fake.accountId));
  });

  test("pending until the server answered", () => {
    const { data } = fakeData();
    const shown = probe(data, () => useSuggestions());
    expect(shown.status).toBe("pending");
    expect(shown.items).toEqual([]);
  });

  test("not enabled, it shows no one", () => {
    const { data } = fakeData();
    expect(probe(data, () => useSuggestions(false)).items).toEqual([]);
  });
});
