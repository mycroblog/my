# Sign-in with external providers and the OAuth flows

- Changing the GitHub login: [GitHub Docs, Authorizing OAuth apps](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps), search for: `https://github.com/login/oauth/authorize`, `https://github.com/login/oauth/access_token`, `scope`, `state`
- Changing the Discord login: [Discord developer documentation, OAuth2](https://docs.discord.com/developers/topics/oauth2), search for: `https://discord.com/oauth2/authorize`, `https://discord.com/api/oauth2/token`, `identify`, `email`
- Changing the login with an OpenID Connect provider: [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html), search for: Authentication using the Authorization Code Flow, `nonce`, `prompt`, Successful Token Response
- Changing the authorization code flow that the server issues tokens with: [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749), search for: Authorization Code Grant, Access Token Response, Refreshing an Access Token
- Changing the handling of a refresh token that was used twice: [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700), search for: Refresh Token Protection, refresh token rotation
