// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { authorizedAppsQuery } from "./authorized-apps";
import { browserSessionsQuery } from "./browser-sessions";
import { changePassword, passwordStatusQuery, removePassword, useHasPassword, usePasswordWrites } from "./password";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, marked, passkeyConfirmation, refusal } from "./test/settings";

/** The data after the sessions and the apps were read, as the screens of the settings hold them. */
async function read(options: Parameters<typeof fakeData>[0]) {
  const given = fakeData(options);
  await given.data.queryClient.query(passwordStatusQuery(given.data));
  await given.data.queryClient.query(browserSessionsQuery(given.data));
  await given.data.queryClient.query(authorizedAppsQuery(given.data));
  return given;
}

describe("useHasPassword", () => {
  test("is undefined until it is read, then the server's answer", async () => {
    const without = fakeData({ access: { password: false } }).data;
    expect(probe(without, useHasPassword)).toBeUndefined();
    await without.queryClient.query(passwordStatusQuery(without));
    expect(probe(without, useHasPassword)).toBe(false);
    const withOne = fakeData().data;
    await withOne.queryClient.query(passwordStatusQuery(withOne));
    expect(probe(withOne, useHasPassword)).toBe(true);
  });
});

describe("changePassword", () => {
  test("sends a password confirmation as `current_password`, and a wrong one rejects with the field it names", async () => {
    const { fake, data } = await read({});
    const sent = fake.script("POST /accounts/password");
    const refused = await refusal(
      changePassword(data, "a new password", { kind: "password", password: "not the password" }),
    );
    expect(sent.seen.map((s) => s.body)).toEqual([
      { new_password: "a new password", current_password: "not the password" },
    ]);
    expect([kindOf(refused), refused.problem.field]).toEqual(["wrong-password", "current_password"]);
    expect(probe(data, useHasPassword)).toBe(true);
    expect(marked(data, browserSessionsQuery(data).queryKey)).toBe(false);
  });

  test("rejects with the field of a new password the server refuses, and changes nothing", async () => {
    const { data } = await read({ access: { password: false } });
    const refused = await refusal(changePassword(data, "short", await passkeyConfirmation(data)));
    expect(refused.problem.field).toBe("new_password");
    expect(probe(data, useHasPassword)).toBe(false);
    expect(marked(data, authorizedAppsQuery(data).queryKey)).toBe(false);
  });

  test("sets a password where there was none, and marks the browsers and the apps, which the server ended", async () => {
    const { data } = await read({ access: { password: false } });
    await changePassword(data, "a new password", await passkeyConfirmation(data));
    expect(probe(data, useHasPassword)).toBe(true);
    expect(marked(data, browserSessionsQuery(data).queryKey)).toBe(true);
    expect(marked(data, authorizedAppsQuery(data).queryKey)).toBe(true);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await read({ access: { password: false } });
    await probe(data, usePasswordWrites).change("a new password", await passkeyConfirmation(data));
    expect(probe(data, useHasPassword)).toBe(true);
  });
});

describe("removePassword", () => {
  test("removes it, and the cache says none, with the browsers and the apps marked", async () => {
    const { data } = await read({});
    await removePassword(data, await passkeyConfirmation(data));
    expect(probe(data, useHasPassword)).toBe(false);
    expect(marked(data, browserSessionsQuery(data).queryKey)).toBe(true);
    expect(marked(data, authorizedAppsQuery(data).queryKey)).toBe(true);
  });

  test("rejects with the last-sign-in-method problem where nothing else signs in, and the cache keeps the password", async () => {
    const { data } = await read({ access: { passkeys: 0, linked: "none" } });
    const refused = await refusal(removePassword(data, { kind: "external", token: "any" }));
    expect(kindOf(refused)).toBe("last-sign-in-method");
    expect(probe(data, useHasPassword)).toBe(true);
    expect(marked(data, browserSessionsQuery(data).queryKey)).toBe(false);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await read({});
    await probe(data, usePasswordWrites).remove(await passkeyConfirmation(data));
    expect(probe(data, useHasPassword)).toBe(false);
  });
});
