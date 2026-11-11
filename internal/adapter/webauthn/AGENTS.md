# WebAuthn relying party (go-webauthn)

- Changing the options of a registration or a sign-in (discoverable credentials, user verification, attestation): [Web Authentication Level 3](https://www.w3.org/TR/webauthn-3/), search for: `residentKey`, `userVerification`, `attestation`, `PublicKeyCredentialCreationOptions`
- Changing what the server verifies in a ceremony: [Web Authentication Level 3](https://www.w3.org/TR/webauthn-3/), search for: Registering a New Credential, Verifying an Authentication Assertion, `signCount`, `rpIdHash`
- Changing the relying party ID or the origins: [Web Authentication Level 3](https://www.w3.org/TR/webauthn-3/), search for: RP ID, `origin`, Related Origin Requests
- Calling the library: [go-webauthn package documentation](https://pkg.go.dev/github.com/go-webauthn/webauthn/webauthn), search for: `BeginDiscoverableLogin`, `FinishRegistration`, `SessionData`, `RPOrigins`
