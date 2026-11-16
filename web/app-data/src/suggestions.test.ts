// SPDX-License-Identifier: AGPL-3.0-only

// The suggested accounts (FR-308): read from the server in its order, the accounts alone; and nothing asked while a
// screen does not want them.

import { describe, expect, test } from "bun:test";
import { suggestionsQuery, useSuggestions } from "./suggestions";
import { fakeAppData, json } from "./test/fake";
import { probe } from "./test/probe";

const account = (id: string) => ({ id, username: id, acct: id, display_name: id, emojis: [] });

/** A server that suggests the featured hana, then sam who posted last, then yuki. */
function server() {
  return fakeAppData((request) =>
    new URL(request.url).pathname === "/api/v1/accounts/suggestions"
      ? json(200, {
          items: [
            { source: "featured", account: account("hana") },
            { source: "recently_active", account: account("sam") },
            { source: "recently_active", account: account("yuki") },
          ],
        })
      : new URL(request.url).pathname === "/api/v1/accounts/relationships"
        ? json(200, { items: [] })
        : json(404, { type: "about:blank", title: "Not Found", status: 404 }),
  );
}

describe("suggestions", () => {
  test("the query reads the suggestions, and the hook shows their accounts in the server's order", async () => {
    const { data, requests } = server();
    await data.queryClient.query(suggestionsQuery(data));
    expect(requests[0]?.pathname).toBe("/api/v1/accounts/suggestions");
    const shown = probe(data, () => useSuggestions());
    expect(shown.status).toBe("success");
    expect(shown.items.map((a) => a.id)).toEqual(["hana", "sam", "yuki"]);
  });

  test("pending until the server answered", () => {
    const { data } = server();
    const shown = probe(data, () => useSuggestions());
    expect(shown.status).toBe("pending");
    expect(shown.items).toEqual([]);
  });

  test("not enabled, it shows no one", () => {
    const { data } = server();
    expect(probe(data, () => useSuggestions(false)).items).toEqual([]);
  });
});
