// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  createOAuthApp,
  deleteOAuthApp,
  issueOAuthAppToken,
  oAuthAppQuery,
  oAuthAppsQuery,
  rotateOAuthAppSecret,
  updateOAuthApp,
  useOAuthApp,
  useOAuthApps,
  useOAuthAppWrites,
  type CreateOAuthApp,
} from "./oauth-apps";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { kindOf, refusal } from "./test/settings";

const request = (more: Partial<CreateOAuthApp> = {}): CreateOAuthApp => ({
  name: "Reading list",
  redirect_uris: ["https://reading.example/callback"],
  scope: "read",
  client_type: "confidential",
  ...more,
});

/** The data after the list was read, and the apps in it: the confidential, the public and the disabled one. */
async function read(options: Parameters<typeof fakeData>[0] = {}) {
  const given = fakeData(options);
  const { items: apps } = await given.data.queryClient.query(oAuthAppsQuery(given.data));
  const of = (type: "confidential" | "public") => apps.find((a) => a.client_type === type && !a.disabled);
  return { ...given, apps, confidential: of("confidential"), public: of("public") };
}

describe("useOAuthApps and useOAuthApp", () => {
  test("are undefined until read, then the reader's apps and the one app", async () => {
    const { data } = fakeData();
    expect(probe(data, useOAuthApps)).toBeUndefined();
    const { items: apps } = await data.queryClient.query(oAuthAppsQuery(data));
    expect(probe(data, useOAuthApps)?.length).toBe(3);
    const id = apps[0]?.id ?? "";
    expect(probe(data, () => useOAuthApp(id))).toBeUndefined();
    await data.queryClient.query(oAuthAppQuery(data, id));
    expect(probe(data, () => useOAuthApp(id))?.id).toBe(id);
  });
});

describe("createOAuthApp", () => {
  test("registers a confidential app, answers its secret once, and both caches hold the app", async () => {
    const { data, apps } = await read();
    const created = await createOAuthApp(data, request());
    expect(created.client_secret).toBeTruthy();
    expect(created.app.name).toBe("Reading list");
    expect(probe(data, useOAuthApps)?.map((a) => a.id)).toContain(created.app.id);
    expect(probe(data, useOAuthApps)?.length).toBe(apps.length + 1);
    expect(probe(data, () => useOAuthApp(created.app.id))?.name).toBe("Reading list");
    // The secret is in no cache.
    expect(JSON.stringify(probe(data, useOAuthApps))).not.toContain(created.client_secret ?? "?");
  });

  test("registers a public app with no secret", async () => {
    const { data } = await read();
    const created = await createOAuthApp(data, request({ client_type: "public" }));
    expect(created.client_secret).toBeUndefined();
  });

  test("rejects with the field of a setting the server refuses, and the list is as it was", async () => {
    const { data, apps } = await read();
    const refused = await refusal(createOAuthApp(data, request({ name: " " })));
    expect(refused.problem.field).toBe("name");
    expect(probe(data, useOAuthApps)?.length).toBe(apps.length);
  });

  test("rejects with the disabled-registration problem where the administrator turned registration off", async () => {
    const { data } = await read({ access: { oauthApps: "disabled" } });
    const refused = await refusal(createOAuthApp(data, request()));
    expect([refused.problem.status, kindOf(refused)]).toEqual([403, "oauth-app-registration-disabled"]);
    expect(probe(data, useOAuthApps)).toEqual([]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data } = await read();
    const created = await probe(data, useOAuthAppWrites).create(request());
    expect(probe(data, useOAuthApps)?.map((a) => a.id)).toContain(created.app.id);
  });
});

describe("updateOAuthApp", () => {
  test("changes it in the list and in the one-app cache", async () => {
    const { data, confidential } = await read();
    const id = confidential?.id ?? "";
    await data.queryClient.query(oAuthAppQuery(data, id));
    await updateOAuthApp(data, id, { name: "Renamed" });
    expect(probe(data, () => useOAuthApp(id))?.name).toBe("Renamed");
    expect(probe(data, useOAuthApps)?.find((a) => a.id === id)?.name).toBe("Renamed");
    expect(probe(data, useOAuthApps)?.length).toBe(3);
  });

  test("rejects with the field of a setting the server refuses, and the caches keep the app as it was", async () => {
    const { data, confidential } = await read();
    const id = confidential?.id ?? "";
    await data.queryClient.query(oAuthAppQuery(data, id));
    const refused = await refusal(updateOAuthApp(data, id, { redirect_uris: [] }));
    expect(refused.problem.field).toBe("redirect_uris");
    expect(probe(data, () => useOAuthApp(id))?.name).toBe(confidential?.name);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data, confidential } = await read();
    const id = confidential?.id ?? "";
    await probe(data, useOAuthAppWrites).update(id, { name: "Renamed" });
    expect(probe(data, useOAuthApps)?.find((a) => a.id === id)?.name).toBe("Renamed");
  });
});

describe("deleteOAuthApp", () => {
  test("deletes it from the list, and the one-app cache no longer holds it", async () => {
    const { data, confidential } = await read();
    const id = confidential?.id ?? "";
    await data.queryClient.query(oAuthAppQuery(data, id));
    await deleteOAuthApp(data, id);
    expect(probe(data, useOAuthApps)?.some((a) => a.id === id)).toBe(false);
    expect(probe(data, () => useOAuthApp(id))).toBeUndefined();
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data, confidential } = await read();
    await probe(data, useOAuthAppWrites).remove(confidential?.id ?? "");
    expect(probe(data, useOAuthApps)?.length).toBe(2);
  });
});

describe("rotateOAuthAppSecret", () => {
  test("answers a new secret, and none is in the caches", async () => {
    const { data, confidential } = await read();
    const secret = await rotateOAuthAppSecret(data, confidential?.id ?? "");
    expect(secret).toBeTruthy();
    expect(JSON.stringify(probe(data, useOAuthApps))).not.toContain(secret);
  });

  test("rejects with the public-app problem for an app that has no secret", async () => {
    const { data, public: open } = await read();
    const refused = await refusal(rotateOAuthAppSecret(data, open?.id ?? ""));
    expect(kindOf(refused)).toBe("public-oauth-app");
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data, confidential } = await read();
    expect(await probe(data, useOAuthAppWrites).rotateSecret(confidential?.id ?? "")).toBeTruthy();
  });
});

describe("issueOAuthAppToken", () => {
  test("issues a token for the app's scope, or for a narrower one that is asked for", async () => {
    const { data, confidential } = await read();
    const id = confidential?.id ?? "";
    const whole = await issueOAuthAppToken(data, id);
    expect(whole.access_token).toBeTruthy();
    expect(whole.scope).toBe(confidential?.scope ?? "");
    const narrow = await issueOAuthAppToken(data, id, "read");
    expect(narrow.scope).toBe("read");
  });

  test("rejects with the field `scope` for a scope beyond the app's", async () => {
    const { data, confidential } = await read();
    const refused = await refusal(issueOAuthAppToken(data, confidential?.id ?? "", "admin"));
    expect([refused.problem.status, refused.problem.field]).toEqual([422, "scope"]);
  });

  test("through the hook, is the same write bound to the session", async () => {
    const { data, confidential } = await read();
    expect((await probe(data, useOAuthAppWrites).issueToken(confidential?.id ?? "")).access_token).toBeTruthy();
  });
});
