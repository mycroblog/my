# ACME client (acmez v3)

- Changing the order, the challenges, or the finalization: [RFC 8555](https://www.rfc-editor.org/rfc/rfc8555), search for: `newOrder`, `finalize`, Identifier Validation Challenges, `badNonce`, `orderNotReady`
- Changing the account or the external account binding: [RFC 8555](https://www.rfc-editor.org/rfc/rfc8555), search for: `newAccount`, `onlyReturnExisting`, External Account Binding, `termsOfServiceAgreed`
- Revoking a certificate: [RFC 8555](https://www.rfc-editor.org/rfc/rfc8555), search for: Certificate Revocation, `revokeCert`, `alreadyRevoked`
- Changing when a certificate renews (ARI): [RFC 9773](https://www.rfc-editor.org/rfc/rfc9773), search for: `renewalInfo`, `suggestedWindow`, `replaces`, `Retry-After`
- Choosing a profile of the CA (an Internet-Draft, not an RFC): [draft-ietf-acme-profiles](https://datatracker.ietf.org/doc/draft-ietf-acme-profiles/), search for: `profiles`, `profile`, `invalidProfile`
- Calling the library: [acmez v3 package documentation](https://pkg.go.dev/github.com/mholt/acmez/v3), search for: `ObtainCertificate`, `Solver`, `OrderParameters`
- Changing the defaults that follow Let's Encrypt (directory URLs, rate limits, profiles): [Let's Encrypt documentation](https://letsencrypt.org/docs/), search for: Staging Environment, Rate Limits, Profiles, ACME Renewal Information
