# Credentials and OAuth rules: TOTP, PKCE, redirect URIs, scopes

- Changing the TOTP check, its time step, or its window: [RFC 6238](https://www.rfc-editor.org/rfc/rfc6238), search for: time step, Validation and Time-Step Size, Resynchronization
- Changing the HOTP value behind a TOTP code: [RFC 4226](https://www.rfc-editor.org/rfc/rfc4226), search for: Dynamic Truncation, Generating an HOTP Value
- Encoding or decoding a TOTP secret: [RFC 4648](https://www.rfc-editor.org/rfc/rfc4648), search for: Base 32 Encoding, padding
- Changing the PKCE transformation or the verifier's alphabet: [RFC 7636](https://www.rfc-editor.org/rfc/rfc7636), search for: `code_verifier`, `S256`, unreserved
- Changing which redirect URIs a client may register or use: [RFC 8252](https://www.rfc-editor.org/rfc/rfc8252), search for: Loopback Interface Redirection, Private-Use URI Scheme Redirection, `127.0.0.1`
- Changing the scope syntax or the grant rules: [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749), search for: Access Token Scope, `scope-token`, Authorization Code Grant
- Changing the client metadata that a registration accepts: [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591), search for: Client Metadata, `redirect_uris`, `client_name`
