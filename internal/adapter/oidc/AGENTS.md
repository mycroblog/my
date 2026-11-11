# OpenID Connect relying party

- Verifying an ID token: [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html), search for: ID Token Validation, `nonce`, `azp`, `auth_time`
- Reading a provider's Discovery document: [OpenID Connect Discovery 1.0](https://openid.net/specs/openid-connect-discovery-1_0.html), search for: `jwks_uri`, `id_token_signing_alg_values_supported`, OpenID Provider Configuration Validation, `/.well-known/openid-configuration`
- Selecting a key of the JWKS: [RFC 7517](https://www.rfc-editor.org/rfc/rfc7517), search for: `kid`, `use`, `key_ops`, `kty`
- Verifying an RS256 or ES256 signature: [RFC 7518](https://www.rfc-editor.org/rfc/rfc7518), search for: `RS256`, `ES256`, RSASSA-PKCS1-v1_5, ECDSA P-256 SHA-256
- Verifying an EdDSA signature or reading an `OKP` key: [RFC 8037](https://www.rfc-editor.org/rfc/rfc8037), search for: `OKP`, `EdDSA`, `Ed25519`
- Parsing the compact serialization or the header of a JWS: [RFC 7515](https://www.rfc-editor.org/rfc/rfc7515), search for: JWS Compact Serialization, `alg`, `crit`
- Validating the claims of a JWT: [RFC 7519](https://www.rfc-editor.org/rfc/rfc7519), search for: `exp`, `aud`, `iss`, `nbf`
