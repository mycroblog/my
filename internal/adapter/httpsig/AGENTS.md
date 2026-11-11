# HTTP signatures: RFC 9421 and draft-cavage-12

- Building a signature base or a `Signature-Input` (RFC 9421): [RFC 9421](https://www.rfc-editor.org/rfc/rfc9421), search for: Creating the Signature Base, `@signature-params`, `Signature-Input`, Derived Components, `keyid`
- Signing or verifying in the draft form (draft-cavage-12, which RFC 9421 replaced and most servers still send): [draft-cavage-http-signatures-12](https://datatracker.ietf.org/doc/html/draft-cavage-http-signatures-12), search for: `(request-target)`, `keyId`, `headers`, `hs2019`, `rsa-sha256`
- Changing the body digest of an RFC 9421 signature: [RFC 9530](https://www.rfc-editor.org/rfc/rfc9530), search for: `Content-Digest`, `sha-256`
- Changing the `Digest` header of a draft signature (RFC 3230, obsoleted by RFC 9530): [RFC 3230](https://www.rfc-editor.org/rfc/rfc3230), search for: `Digest`, `SHA`
- Parsing or writing the structured fields of `Signature` and `Signature-Input`: [RFC 9651](https://www.rfc-editor.org/rfc/rfc9651), search for: Dictionaries, Inner Lists, Byte Sequences, Parameters
