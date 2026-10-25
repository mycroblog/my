// SPDX-License-Identifier: MIT
import {
  codeChallenge,
  createClient,
  exchange,
  newCodeVerifier,
  RefreshingTokenSource,
  type Api,
  type OAuthConfig,
  type Token,
} from "@mycroblog/mykit";

const oauth: OAuthConfig = {
  server: "https://example.com",
  clientId: "https://app.example/client.json", // a Client ID Metadata Document URL, or a registered client_id
  redirectUri: "http://127.0.0.1:8123/callback",
};

// Step 1: send the user to the authorization endpoint with the PKCE challenge. Keep the verifier for step 2.
export async function authorizationUrl(): Promise<{ url: string; verifier: string }> {
  const verifier = newCodeVerifier();
  const query = new URLSearchParams({
    response_type: "code",
    client_id: oauth.clientId,
    redirect_uri: oauth.redirectUri ?? "",
    scope: "read write",
    code_challenge: await codeChallenge(verifier),
    code_challenge_method: "S256",
  });
  return { url: `${oauth.server}/oauth/authorize?${query.toString()}`, verifier };
}

// The tokens renew themselves. The server rotates the refresh token on every use, so onTokens must save the new
// tokens: a lost refresh token ends the session.
export function clientFor(saved: Token, save: (token: Token) => Promise<void>): Api {
  return createClient({ server: oauth.server, tokenSource: new RefreshingTokenSource(oauth, saved, save) });
}

// Step 2: exchange the code that the redirect brought for the tokens, save them, and return a client that uses them.
export async function signIn(code: string, verifier: string, save: (token: Token) => Promise<void>): Promise<Api> {
  const token = await exchange(oauth, code, verifier);
  await save(token);
  return clientFor(token, save);
}
