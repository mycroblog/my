// SPDX-License-Identifier: AGPL-3.0-only
// Helpers of the demo server's specs (e2e/demo-*.spec.ts, ADR-0117): the people of the demo scenario, the client's
// messages, signing in through the app, and the client API as one of the people.
import { createHash, randomBytes } from "node:crypto";
import { type APIRequestContext, type APIResponse, type Page, request } from "@playwright/test";
import { personas, type Persona } from "../demo/scenario.ts";
import { expect } from "./fixtures";
import { readCatalog } from "./catalog.ts";
import { demoServerURL } from "./server-origin.ts";

/** The client message `key` (the project is in English). */
export function message(key: string): string {
  const text = readCatalog("en")[key]?.defaultMessage;
  if (text === undefined) throw new Error(`no message ${key}`);
  return text;
}

/** The person of the demo scenario with `username`. */
export function persona(username: string): Persona {
  const p = personas.find((x) => x.username === username);
  if (p === undefined) throw new Error(`no persona ${username}`);
  return p;
}

/** Signs in to the app through the server's sign-in screen, which /login opens at once, and waits for the home page. */
export async function signIn(page: Page, email: string, password: string): Promise<void> {
  await page.goto("/login");
  await page.locator('input[type="email"][autocomplete="username"]').fill(email);
  await page.locator('input[type="password"][autocomplete="current-password"]').fill(password);
  await page.locator('main button[type="submit"]').first().click();
  await expect(page).toHaveURL(`${demoServerURL}/home`);
  await expect(page.getByRole("main").getByRole("heading", { level: 1, name: message("home.title") })).toBeVisible();
}

/** The Location of the redirect `res`, resolved against the demo server. */
function location(res: APIResponse): string {
  const to = res.headers()["location"];
  if (to === undefined || res.status() < 300 || res.status() >= 400) {
    throw new Error(`expected a redirect from ${res.url()}, got ${String(res.status())}`);
  }
  return new URL(to, demoServerURL).href;
}

/** The client API of the demo server as one person: `call` sends a request under /api/v1 with the person's token. */
export type DemoAPI = {
  call: (method: "GET" | "POST", path: string, body?: unknown) => Promise<unknown>;
  dispose: () => Promise<void>;
};

/**
 * Signs in as `p` through the web client's authorization code flow with PKCE, as demo/seed.ts does, and returns the
 * client API with the person's access token.
 */
export async function apiAs(p: Persona): Promise<DemoAPI> {
  const http: APIRequestContext = await request.newContext({ baseURL: demoServerURL, maxRedirects: 0 });
  const redirectUri = `${demoServerURL}/login/callback`;
  const verifier = randomBytes(32).toString("base64url");
  const authorize = `/oauth/authorize?${new URLSearchParams({
    response_type: "code",
    client_id: "my-web",
    redirect_uri: redirectUri,
    state: "e2e",
    code_challenge: createHash("sha256").update(verifier).digest("base64url"),
    code_challenge_method: "S256",
  }).toString()}`;
  const signInPage = location(await http.get(authorize));
  const csrf = /name="csrf_token" value="([^"]+)"/.exec(await (await http.get(signInPage)).text())?.[1];
  if (csrf === undefined) throw new Error("the sign-in page has no csrf_token");
  const signedIn = await http.post("/auth/sign_in", {
    form: {
      csrf_token: csrf,
      email: p.email,
      password: p.password,
      return_to: new URL(signInPage).searchParams.get("return_to") ?? "",
    },
  });
  expect(signedIn.status(), `signing in as ${p.username}`).toBe(303);
  const code = new URL(location(await http.get(authorize))).searchParams.get("code");
  if (code === null) throw new Error(`the authorization of ${p.username} returned no code`);
  const token = await http.post("/oauth/token", {
    form: {
      grant_type: "authorization_code",
      code,
      code_verifier: verifier,
      redirect_uri: redirectUri,
      client_id: "my-web",
    },
  });
  expect(token.status()).toBe(200);
  const { access_token: accessToken } = (await token.json()) as { access_token: string };
  return {
    call: async (method, path, body) => {
      const res = await http.fetch(`/api/v1${path}`, {
        method,
        headers: { Authorization: `Bearer ${accessToken}` },
        ...(body === undefined ? {} : { data: body }),
      });
      expect(res.ok(), `${method} ${path} answered ${String(res.status())}: ${await res.text()}`).toBe(true);
      return res.status() === 204 ? undefined : ((await res.json()) as unknown);
    },
    dispose: () => http.dispose(),
  };
}
