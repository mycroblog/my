# OAuth 2.0 authorization server: token, revocation, metadata, registration

- Changing the token endpoint or its errors: [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749), search for: Token Endpoint, `invalid_grant`, `invalid_client`, Refreshing an Access Token, Client Password
- Changing the PKCE check: [RFC 7636](https://www.rfc-editor.org/rfc/rfc7636), search for: `code_verifier`, `code_challenge_method`, `S256`
- Changing revocation: [RFC 7009](https://www.rfc-editor.org/rfc/rfc7009), search for: `token_type_hint`, `unsupported_token_type`, Revocation Response
- Changing the authorization server metadata: [RFC 8414](https://www.rfc-editor.org/rfc/rfc8414), search for: `/.well-known/oauth-authorization-server`, `code_challenge_methods_supported`, `revocation_endpoint`
- Changing the protected resource metadata or the `WWW-Authenticate` challenge: [RFC 9728](https://www.rfc-editor.org/rfc/rfc9728), search for: `/.well-known/oauth-protected-resource`, `resource_metadata`, `authorization_servers`
- Changing dynamic client registration: [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591), search for: `redirect_uris`, `token_endpoint_auth_method`, `invalid_redirect_uri`, `client_secret_expires_at`
- Changing the read of a registration (client configuration endpoint): [RFC 7592](https://www.rfc-editor.org/rfc/rfc7592), search for: `registration_access_token`, `registration_client_uri`
- Accepting a URL as a `client_id` (an Internet-Draft, not an RFC): [draft-ietf-oauth-client-id-metadata-document](https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/), search for: `client_id_metadata_document_supported`, Client ID Metadata Document
- Changing the rotation or the reuse detection of refresh tokens: [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700), search for: Refresh Token Protection, refresh token rotation
