// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, onTestFinished, test } from "bun:test";
import { fakeOrigin, scenario } from "@my/e2e/fake";
import { QueryObserver } from "@tanstack/react-query";
import { viewerQuery } from "./accounts";
import {
  accountAliasesQuery,
  addAccountAlias,
  cancelOwnAccountMove,
  moveOwnAccount,
  moveRefusalOf,
  nextMoveAt,
  removeAccountAlias,
  useAccountAliases,
} from "./account-move";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";
import { settled } from "./test/settings";

/** The password of the reader (yuki), which confirms a move. */
const password = scenario.people.find((p) => p.username === "yuki")?.password ?? "";

describe("the account's aliases", () => {
  test("are none until read, then the server's with their handle and URI", async () => {
    const { data } = fakeData();
    expect(probe(data, useAccountAliases)).toEqual([]);
    await data.queryClient.query(accountAliasesQuery(data));
    expect(probe(data, useAccountAliases).map((a) => [a.acct, a.uri])).toEqual([
      ["yuki@old.example", "https://old.example/users/yuki"],
    ]);
  });

  test("an alias added is kept in the cache as the server answered, and the others stay", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("POST /accounts/aliases");
    await data.queryClient.query(accountAliasesQuery(data));
    await addAccountAlias(data, "@yuki@new.example");
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["POST /accounts/aliases"]);
    expect(sent.seen).toHaveLength(1);
    expect(probe(data, useAccountAliases).map((a) => a.acct)).toEqual(["yuki@old.example", "yuki@new.example"]);
  });

  test("an alias given by its profile's address is the same alias as by its handle, and is kept once", async () => {
    const { data } = fakeData();
    await data.queryClient.query(accountAliasesQuery(data));
    await addAccountAlias(data, "https://old.example/users/yuki");
    expect(probe(data, useAccountAliases).map((a) => a.acct)).toEqual(["yuki@old.example"]);
  });

  test("an alias removed leaves the cache, by its URI", async () => {
    const { data, requests } = fakeData();
    await data.queryClient.query(accountAliasesQuery(data));
    const [alias] = probe(data, useAccountAliases);
    await removeAccountAlias(data, alias?.uri ?? "");
    expect(requests.at(-1)).toBe(`DELETE /accounts/aliases?uri=${encodeURIComponent(alias?.uri ?? "")}`);
    expect(probe(data, useAccountAliases)).toEqual([]);
  });

  test("an account that cannot be found is refused with the problem that says so, and the cache stays", async () => {
    const { data } = fakeData();
    await data.queryClient.query(accountAliasesQuery(data));
    const refused = await refusedWith(addAccountAlias(data, "not an address"));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/invalid-alias"]);
    expect(probe(data, useAccountAliases)).toHaveLength(1);
  });

  test("one more than the limit is refused with the problem that says so", async () => {
    const { data } = fakeData({
      settings: { aliases: ["a@one.example", "a@two.example", "a@three.example", "a@four.example", "a@five.example"] },
    });
    await data.queryClient.query(accountAliasesQuery(data));
    const refused = await refusedWith(addAccountAlias(data, "a@six.example"));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/too-many-aliases"]);
    expect(probe(data, useAccountAliases)).toHaveLength(5);
  });
});

describe("moveOwnAccount", () => {
  test("moves with a password, a passkey or an external confirmation, and marks the viewer stale", async () => {
    for (const proof of [{ password }, { passkey: { id: "key" } }, { external_confirmation: "confirmed" }] as const) {
      const { fake, data, requests } = fakeData();
      const sent = fake.script("POST /accounts/move");
      await data.queryClient.query(viewerQuery(data));
      await moveOwnAccount(data, fake.accountId("marco"), proof as never);
      expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["POST /accounts/move"]);
      expect(sent.seen[0]?.body).toMatchObject({ target_account_id: fake.accountId("marco") });
      expect(data.queryClient.getQueryState(viewerQuery(data).queryKey)?.isInvalidated).toBe(true);
    }
  });

  test("a wrong password is refused with the problem a screen tells apart, and the viewer stays", async () => {
    const { fake, data } = fakeData();
    await data.queryClient.query(viewerQuery(data));
    const refused = await refusedWith(moveOwnAccount(data, fake.accountId("marco"), { password: "wrong" }));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/wrong-password"]);
    expect(data.queryClient.getQueryState(viewerQuery(data).queryKey)?.isInvalidated).toBe(false);
  });

  test("a target without the account as its alias is refused with the problem that says so", async () => {
    const { fake, data } = fakeData({ settings: { moveRefusal: "alias_missing" } });
    const refused = await refusedWith(moveOwnAccount(data, fake.accountId("marco"), { password }));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/move-alias-missing"]);
  });

  test("an account that does not exist is refused as a target that cannot be moved to", async () => {
    const { data } = fakeData();
    const refused = await refusedWith(moveOwnAccount(data, "gone", { password }));
    expect([refused.status, refused.type]).toEqual([422, "/api/v1/problems/move-target-unavailable"]);
  });

  for (const [reason, type] of [
    ["too_soon", "/api/v1/problems/move-too-soon"],
    ["repeated", "/api/v1/problems/move-repeated"],
  ] as const)
    test(`a move refused as ${reason} comes with its own problem, which moveRefusalOf tells apart`, async () => {
      const { fake, data } = fakeData({ settings: { moveRefusal: reason } });
      const refused = await refusedWith(moveOwnAccount(data, fake.accountId("marco"), { password }));
      expect(refused.type).toBe(type);
      expect(moveRefusalOf(refused)).toBe(reason);
    });

  test("a second move right after the first is refused as too soon", async () => {
    const { fake, data } = fakeData();
    await moveOwnAccount(data, fake.accountId("marco"), { password });
    await cancelOwnAccountMove(data);
    const refused = await refusedWith(moveOwnAccount(data, fake.accountId("sam"), { password }));
    expect(moveRefusalOf(refused)).toBe("too_soon");
  });

  test("refused as too soon, reads the viewer again before it rejects, which then says from when it can move", async () => {
    const { fake, data, requests } = fakeData({ settings: { moveRefusal: "too_soon" } });
    const before = await data.queryClient.query(viewerQuery(data));
    expect(before.can_move_at).toBeUndefined();
    // A screen shows the viewer.
    onTestFinished(new QueryObserver(data.queryClient, viewerQuery(data)).subscribe(() => undefined));

    await refusedWith(moveOwnAccount(data, fake.accountId("marco"), { password }));

    // What the cache holds as the refusal arrives.
    const account = data.queryClient.getQueryData<{ can_move_at?: string; last_moved_at?: string }>(
      viewerQuery(data).queryKey,
    );
    expect(requests.slice(0, 3)).toEqual([
      "GET /accounts/verify_credentials",
      "POST /accounts/move",
      "GET /accounts/verify_credentials",
    ]);
    expect(account?.can_move_at).toBeString();
    expect(account?.last_moved_at).toBeString();
  });
});

describe("nextMoveAt", () => {
  test("gives can_move_at while it is later than now, and nothing at it or after it", async () => {
    const { data } = fakeData({ settings: { lastMoved: "-12d" } });
    const account = await data.queryClient.query(viewerQuery(data));
    const at = account.can_move_at ?? "";
    const when = Date.parse(at);
    expect(Number.isNaN(when)).toBe(false);

    expect(nextMoveAt(account, new Date(when - 1))).toBe(at);
    expect(nextMoveAt(account, new Date(when))).toBeUndefined();
    expect(nextMoveAt(account, new Date(when + 1))).toBeUndefined();
  });

  test("measures against the present by default", async () => {
    const { data } = fakeData({ settings: { lastMoved: "-12d" } });
    const account = await data.queryClient.query(viewerQuery(data));
    expect(nextMoveAt(account)).toBe(account.can_move_at ?? "");
    expect(nextMoveAt({ ...account, can_move_at: new Date(Date.now() - 1000).toISOString() })).toBeUndefined();
  });

  test("gives nothing to an account that can move now", async () => {
    const { data } = fakeData();
    const account = await data.queryClient.query(viewerQuery(data));
    expect(account.can_move_at).toBeUndefined();
    expect(nextMoveAt(account)).toBeUndefined();
  });
});

describe("cancelOwnAccountMove", () => {
  test("cancels a move that was made, and marks the viewer stale", async () => {
    const { fake, data, requests } = fakeData();
    await moveOwnAccount(data, fake.accountId("marco"), { password });
    await data.queryClient.query(viewerQuery(data));
    await cancelOwnAccountMove(data);
    expect(requests.at(-1)).toBe("DELETE /accounts/move");
    expect(data.queryClient.getQueryState(viewerQuery(data).queryKey)?.isInvalidated).toBe(true);
  });

  test("a move that was not made (a 409: the account has not moved) resolves, and the viewer is read again", async () => {
    const { fake, data, requests } = fakeData();
    // The server's answer, asked as a client does: the account has not moved.
    const asked = await fake.serve(new Request(`${fakeOrigin}/api/v1/accounts/move`, { method: "DELETE" }));
    const problem: unknown = await asked.json();
    expect([asked.status, problem]).toMatchObject([409, { type: "/api/v1/problems/account-not-moved" }]);
    await data.queryClient.query(viewerQuery(data));
    // A screen shows the viewer, which says whether the account moved.
    onTestFinished(new QueryObserver(data.queryClient, viewerQuery(data)).subscribe(() => undefined));

    await cancelOwnAccountMove(data);
    await settled(data);

    expect(requests).toEqual([
      "GET /accounts/verify_credentials",
      "DELETE /accounts/move",
      "GET /accounts/verify_credentials",
    ]);
  });
});
