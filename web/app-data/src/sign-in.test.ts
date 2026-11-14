// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { codeChallenge } from "@mycroblog/mykit";
import { appPath, finishSignIn, SignInError, startSignIn, type SignInOptions } from "./sign-in";

/** A tab's storage. */
function storage() {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => void items.set(key, value),
    removeItem: (key: string) => void items.delete(key),
  };
}

/** A token endpoint that answers `status`, and the forms it got. */
function tokenEndpoint(status = 200) {
  const forms: URLSearchParams[] = [];
  const fetch = async (request: Request) => {
    expect(new URL(request.url).pathname).toBe("/oauth/token");
    forms.push(new URLSearchParams(await request.text()));
    return status === 200
      ? Response.json({ access_token: "my_at_x", token_type: "Bearer", expires_in: 3600, scope: "read" })
      : Response.json({ error: "invalid_grant" }, { status });
  };
  return { forms, fetch };
}

const options = (s: ReturnType<typeof storage>, fetch?: SignInOptions["fetch"]): SignInOptions => ({
  server: "https://my.example",
  clientId: "my-web",
  redirectUri: "https://my.example/login/callback",
  storage: s,
  ...(fetch ? { fetch } : {}),
});

test("the start asks the authorization endpoint for a code with PKCE and keeps its state and verifier", async () => {
  const s = storage();
  const url = new URL(await startSignIn(options(s), "/notifications?x=1"));
  expect(url.origin + url.pathname).toBe("https://my.example/oauth/authorize");
  const q = url.searchParams;
  expect(q.get("response_type")).toBe("code");
  expect(q.get("client_id")).toBe("my-web");
  expect(q.get("redirect_uri")).toBe("https://my.example/login/callback");
  expect(q.get("code_challenge_method")).toBe("S256");
  const kept = JSON.parse(s.items.get("my.sign-in") ?? "{}") as { state: string; verifier: string; returnTo: string };
  expect(q.get("state")).toBe(kept.state);
  expect(q.get("code_challenge")).toBe(await codeChallenge(kept.verifier));
  expect(kept.returnTo).toBe("/notifications?x=1");
});

test("the callback trades the code with the verifier and goes back to where the sign-in started", async () => {
  const s = storage();
  const state = new URL(await startSignIn(options(s), "/welcome")).searchParams.get("state") ?? "";
  const { verifier } = JSON.parse(s.items.get("my.sign-in") ?? "{}") as { verifier: string };
  const token = tokenEndpoint();
  const callback = new URL(
    `https://my.example/login/callback?code=my_ac_1&state=${state}&iss=https%3A%2F%2Fmy.example`,
  );
  expect(await finishSignIn(options(s, token.fetch), callback)).toBe("/welcome");
  expect(token.forms.map((f) => Object.fromEntries(f))).toEqual([
    {
      grant_type: "authorization_code",
      code: "my_ac_1",
      code_verifier: verifier,
      redirect_uri: "https://my.example/login/callback",
      client_id: "my-web",
    },
  ]);
  // A callback is taken once.
  expect(s.items.size).toBe(0);
  expect(finishSignIn(options(s, token.fetch), callback)).rejects.toEqual(new SignInError("stale"));
});

test("the callback refuses an answer that is not of the start", async () => {
  const cases: [string, string, SignInError["reason"]][] = [
    ["another state", "code=c&state=other", "stale"],
    ["another issuer", "code=c&state=STATE&iss=https%3A%2F%2Fevil.example", "issuer"],
    ["an error", "error=access_denied&state=STATE", "denied"],
    ["no code", "state=STATE", "denied"],
  ];
  for (const [name, query, reason] of cases) {
    const s = storage();
    const state = new URL(await startSignIn(options(s))).searchParams.get("state") ?? "";
    const token = tokenEndpoint();
    const callback = new URL(`https://my.example/login/callback?${query.replace("STATE", state)}`);
    const error = await finishSignIn(options(s, token.fetch), callback).catch((e: unknown) => e);
    expect(error, name).toBeInstanceOf(SignInError);
    expect((error as SignInError).reason, name).toBe(reason);
    expect(token.forms, name).toHaveLength(0);
  }
});

test("a code the token endpoint refuses fails the sign-in", async () => {
  const s = storage();
  const state = new URL(await startSignIn(options(s))).searchParams.get("state") ?? "";
  const callback = new URL(`https://my.example/login/callback?code=c&state=${state}`);
  const error = await finishSignIn(options(s, tokenEndpoint(400).fetch), callback).catch((e: unknown) => e);
  expect((error as SignInError).reason).toBe("exchange");
});

test("the sign-in goes back only to a path of the app's own origin", () => {
  expect(appPath("/notifications")).toBe("/notifications");
  for (const other of [undefined, null, "", "https://evil.example/", "//evil.example/", "/\\evil.example", "welcome"])
    expect(appPath(other)).toBe("/");
});
