// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { fakeExternalConfirmation } from "@my/e2e/fake";
import type { AppData } from "./app-data";
import {
  beginExternalConfirmation,
  beginLinkIdentity,
  linkedIdentitiesQuery,
  readExternalOutcome,
  unlinkIdentity,
  useLinkedIdentities,
  useLinkedIdentityWrites,
  type ExternalOutcome,
} from "./linked-identities";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, passkeyConfirmation, refusal } from "./test/settings";

/** What a screen shown again shows: the cache while it holds the answer and it is not marked, else a read. */
const shown = (data: AppData) => data.queryClient.query({ ...linkedIdentitiesQuery(data), staleTime: Infinity });

describe("readExternalOutcome", () => {
  // What the optional parts of an outcome hold when the fragment names none is not pinned: `toEqual` reads an absent
  // one and an undefined one alike.
  const table: readonly (readonly [string, string, Partial<ExternalOutcome> | null])[] = [
    ["a link", "#linked=p1&provider=p1", { kind: "linked", provider: "p1" }],
    ["a link, without the hash", "linked=p1&provider=p1", { kind: "linked", provider: "p1" }],
    [
      "a confirmation",
      "#external_confirmation=tok&provider=p1&operation=password_removal",
      { kind: "confirmation", token: "tok", provider: "p1", operation: "password_removal" },
    ],
    [
      "a confirmation of no provider or operation",
      "#external_confirmation=tok",
      { kind: "confirmation", token: "tok" },
    ],
    [
      "an error",
      "#external_error=mismatch&provider=p1&operation=link",
      { kind: "error", reason: "mismatch", provider: "p1", operation: "link" },
    ],
    ["an error of no provider or operation", "#external_error=denied", { kind: "error", reason: "denied" }],
    ["a reason no client knows, read as a failure", "#external_error=teapot", { kind: "error", reason: "failed" }],
    ["a fragment of something else", "#section-2", null],
    ["an empty fragment", "", null],
    ["only a hash", "#", null],
  ];

  for (const [name, fragment, outcome] of table)
    test(`reads ${name}`, () => {
      expect(readExternalOutcome(fragment)).toEqual(outcome as ExternalOutcome | null);
    });

  for (const reason of [
    "rate_limited",
    "unavailable",
    "expired",
    "denied",
    "mismatch",
    "linked_elsewhere",
    "already_linked",
    "failed",
  ] as const)
    test(`reads the reason ${reason} as itself`, () => {
      const outcome = readExternalOutcome(`#external_error=${reason}`);
      expect(outcome?.kind === "error" ? outcome.reason : outcome).toBe(reason);
    });
});

describe("useLinkedIdentities", () => {
  test("is undefined until the list is read, then the links and the providers", async () => {
    const { data } = fakeData();
    expect(probe(data, useLinkedIdentities)).toBeUndefined();
    await data.queryClient.query(linkedIdentitiesQuery(data));
    const read = probe(data, useLinkedIdentities);
    expect([read?.items.length, read?.providers.length]).toEqual([1, 3]);
  });
});

describe("beginLinkIdentity", () => {
  test("answers where the browser goes, with the confirmation given", async () => {
    const { data } = fakeData({ access: { linked: "none" } });
    const { providers } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const provider = providers.find((p) => p.allow_link);
    const started = await beginLinkIdentity(data, provider?.id ?? "", await passkeyConfirmation(data));
    expect(started.url).toContain(provider?.id ?? "?");
  });

  test("rejects with the unavailable problem for a provider that allows no link", async () => {
    const { data } = fakeData();
    const { providers } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const closed = providers.find((p) => !p.allow_link);
    const refused = await refusal(beginLinkIdentity(data, closed?.id ?? "", { kind: "external", token: "x" }));
    expect(kindOf(refused)).toBe("login-provider-unavailable");
  });

  test("rejects with the linked problem for a provider the account links already", async () => {
    const { data } = fakeData();
    const { items } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const refused = await refusal(
      beginLinkIdentity(data, items[0]?.provider_id ?? "", { kind: "external", token: fakeExternalConfirmation }),
    );
    expect(kindOf(refused)).toBe("login-provider-linked");
  });

  test("rejects with the wrong-password problem and its field for a wrong password", async () => {
    const { data } = fakeData({ access: { linked: "none" } });
    const { providers } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const refused = await refusal(
      beginLinkIdentity(data, providers[0]?.id ?? "", { kind: "password", password: "not the password" }),
    );
    expect([kindOf(refused), refused.problem.field]).toEqual(["wrong-password", "password"]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = fakeData({ access: { linked: "none" } });
    const { providers } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const started = await probe(data, useLinkedIdentityWrites).beginLink(
      providers[0]?.id ?? "",
      await passkeyConfirmation(data),
    );
    expect(started.url).not.toBe("");
  });
});

describe("beginExternalConfirmation", () => {
  test("answers where the browser goes to confirm an operation at a provider the account links", async () => {
    const { data } = fakeData();
    const { items } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const started = await beginExternalConfirmation(data, items[0]?.provider_id ?? "", "password_removal");
    expect(started.url).toContain("password_removal");
  });

  test("rejects with the unavailable problem for a provider the account links no account at", async () => {
    const { data } = fakeData({ access: { linked: "none" } });
    const { providers } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const refused = await refusal(beginExternalConfirmation(data, providers[0]?.id ?? "", "link"));
    expect(kindOf(refused)).toBe("login-provider-unavailable");
  });
});

describe("unlinkIdentity", () => {
  test("unlinks it, and the list a screen shows again no longer holds it", async () => {
    const { data } = fakeData();
    const { items } = await data.queryClient.query(linkedIdentitiesQuery(data));
    await unlinkIdentity(data, items[0]?.id ?? "");
    expect((await shown(data)).items).toEqual([]);
    expect(probe(data, useLinkedIdentities)?.items).toEqual([]);
  });

  test("rejects with the last-sign-in-method problem for the only way to sign in, and the list keeps it", async () => {
    const { data } = fakeData({ access: { password: false, passkeys: 0 } });
    const { items } = await data.queryClient.query(linkedIdentitiesQuery(data));
    const refused = await refusal(unlinkIdentity(data, items[0]?.id ?? ""));
    expect(kindOf(refused)).toBe("last-sign-in-method");
    expect((await shown(data)).items.map((i) => i.id)).toEqual([items[0]?.id ?? ""]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = fakeData();
    const { items } = await data.queryClient.query(linkedIdentitiesQuery(data));
    await probe(data, useLinkedIdentityWrites).unlink(items[0]?.id ?? "");
    expect((await shown(data)).items).toEqual([]);
  });
});
