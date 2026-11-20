// SPDX-License-Identifier: AGPL-3.0-only

// Signing in as a person of the scenario without a browser, as the web client does (ADR-0090 §16): the authorization
// code flow with PKCE through the server's sign-in screen, its two-factor step when the person has one, and the token
// endpoint. It returns a client of the API with the person's access token, or the web client's session for a browser:
// the refresh token the token endpoint sets as its cookie.

import { codeChallenge, createClient, exchange, newCodeVerifier, staticToken, type Api } from "@mycroblog/mykit";
import type { Person } from "../scenario";
import { totp, totpCode } from "./totp";

/** A cookie jar and fetch without redirects: what a browser does on the server's auth screens. */
class Browser {
  readonly #cookies = new Map<string, string>();
  readonly #origin: string;
  readonly #from: string | undefined;

  constructor(origin: string, from?: string) {
    this.#origin = origin;
    this.#from = from;
  }

  async request(url: string, form?: Record<string, string>): Promise<Response> {
    const headers = new Headers();
    if (this.#from !== undefined) headers.set("X-Forwarded-For", this.#from);
    if (this.#cookies.size > 0) headers.set("Cookie", [...this.#cookies].map(([k, v]) => `${k}=${v}`).join("; "));
    if (form !== undefined) {
      headers.set("Content-Type", "application/x-www-form-urlencoded");
      headers.set("Origin", this.#origin);
      headers.set("Sec-Fetch-Site", "same-origin");
    }
    const res = await fetch(new URL(url, this.#origin), {
      method: form === undefined ? "GET" : "POST",
      headers,
      redirect: "manual",
      ...(form === undefined ? {} : { body: new URLSearchParams(form).toString() }),
    });
    for (const cookie of res.headers.getSetCookie()) {
      const [pair = ""] = cookie.split(";");
      const at = pair.indexOf("=");
      if (at > 0) this.#cookies.set(pair.slice(0, at).trim(), pair.slice(at + 1).trim());
    }
    return res;
  }

  /** The Location of the redirect `res`, as an absolute URL. */
  location(res: Response): string {
    const to = res.headers.get("Location");
    if (to === null || res.status < 300 || res.status >= 400)
      throw new Error(`seed: expected a redirect from ${res.url}, got ${res.status}`);
    return new URL(to, this.#origin).toString();
  }
}

/** The data of an auth screen: the JSON its page carries for the web client's auth entry. */
async function screen(res: Response): Promise<Record<string, unknown>> {
  const html = await res.text();
  const json = /<script type="application\/json" id="auth">(.*?)<\/script>/s.exec(html)?.[1];
  if (json === undefined) throw new Error(`seed: ${res.url} is not an auth screen (${res.status})`);
  return JSON.parse(json) as Record<string, unknown>;
}

/** Submits the form of the auth screen at `url` with its CSRF token and return path, and the `fields`. */
async function submit(browser: Browser, url: string, action: string, fields: Record<string, string>) {
  const data = await screen(await browser.request(url));
  const csrf = data["csrfToken"];
  if (typeof csrf !== "string") throw new Error(`seed: the screen at ${url} has no CSRF token`);
  const returnTo = typeof data["returnTo"] === "string" ? data["returnTo"] : "";
  return browser.request(action, { csrf_token: csrf, return_to: returnTo, ...fields });
}

/** The cookie of the web client's session, which carries its refresh token (internal/interface/oauth). */
export const sessionCookie = "__Host-my_rt";

/**
 * What a sign-in gives: the access token, the refresh token of its cookie (empty when the token endpoint set none), and
 * the fetch that sends from `from`.
 */
type SignedIn = {
  accessToken: string;
  refreshToken: string;
  fetchFrom: { fetch?: (request: Request) => Promise<Response> };
};

/**
 * Signs in as `p` through the authorization code flow, as the web client does. `totpSecret` answers the two-factor
 * step of a person who has one, with a code of a step other than `avoidStep`, one whose code was used already
 * (totpCode). With `from`, every request comes from that client IP (X-Forwarded-For, which the demo server trusts from
 * this machine), so that the people of the E2E tests do not share one sign-in limit (fresh.ts).
 */
async function authorize(
  server: string,
  p: Person,
  totpSecret?: string,
  avoidStep?: number,
  from?: string,
): Promise<SignedIn> {
  const browser = new Browser(server, from);
  const fetchFrom =
    from === undefined
      ? {}
      : {
          fetch: (request: Request) => {
            request.headers.set("X-Forwarded-For", from);
            return fetch(request);
          },
        };
  const redirectUri = `${server}/login/callback`;
  const verifier = newCodeVerifier();
  const authorize = `/oauth/authorize?${new URLSearchParams({
    response_type: "code",
    client_id: "my-web",
    redirect_uri: redirectUri,
    state: "seed",
    code_challenge: await codeChallenge(verifier),
    code_challenge_method: "S256",
  }).toString()}`;
  const signInPage = browser.location(await browser.request(authorize));
  let next = await submit(browser, signInPage, "/auth/sign_in", { email: p.email, password: p.password });
  if (next.status === 303 && browser.location(next).includes("/auth/sign_in/two_factor")) {
    if (totpSecret === undefined) throw new Error(`seed: ${p.username} asks for a two-factor code the seed lacks`);
    const step = browser.location(next);
    next = await submit(browser, step, "/auth/sign_in/two_factor", { code: totpCode(totpSecret, { avoidStep }) });
    // A code signs in once: when this step's code was used already, the next one comes within 30 seconds.
    if (next.status === 422) {
      await new Promise((done) => setTimeout(done, 30_000 - (Date.now() % 30_000) + 500));
      next = await submit(browser, step, "/auth/sign_in/two_factor", { code: totp(totpSecret) });
    }
  }
  if (next.status !== 303) throw new Error(`seed: signing in as ${p.username} answered ${next.status}`);
  const code = new URL(browser.location(await browser.request(authorize))).searchParams.get("code");
  if (code === null) throw new Error(`seed: the authorization of ${p.username} returned no code`);
  let refreshToken = "";
  const send = fetchFrom.fetch ?? ((request: Request) => fetch(request));
  const token = await exchange(
    {
      server,
      clientId: "my-web",
      redirectUri,
      fetch: async (request) => {
        const res = await send(request);
        for (const cookie of res.headers.getSetCookie()) {
          const [pair = ""] = cookie.split(";");
          if (pair.startsWith(`${sessionCookie}=`)) refreshToken = pair.slice(sessionCookie.length + 1);
        }
        return res;
      },
    },
    code,
    verifier,
  );
  return { accessToken: token.accessToken, refreshToken, fetchFrom };
}

/** Signs in as `p` (authorize) and returns a client of the API as them. */
export async function signIn(
  server: string,
  p: Person,
  totpSecret?: string,
  avoidStep?: number,
  from?: string,
): Promise<Api> {
  const { accessToken, fetchFrom } = await authorize(server, p, totpSecret, avoidStep, from);
  return createClient({ server, tokenSource: staticToken(accessToken), acceptLanguage: p.locale, ...fetchFrom });
}

/**
 * Signs in as `p` (authorize) for a browser: the value of the web client's session cookie, which a browser that holds
 * it opens the app with, signed in, as after the sign-in screen. Each call is a session of its own.
 */
export async function webSession(
  server: string,
  p: Person,
  totpSecret?: string,
  avoidStep?: number,
  from?: string,
): Promise<string> {
  const { refreshToken } = await authorize(server, p, totpSecret, avoidStep, from);
  if (refreshToken === "") throw new Error(`seed: the token endpoint set no ${sessionCookie} for ${p.username}`);
  return refreshToken;
}
