# TLS listener and ACME challenge answers

- Answering a TLS-ALPN-01 challenge in the handshake: [RFC 8737](https://www.rfc-editor.org/rfc/rfc8737), search for: `acme-tls/1`, `id-pe-acmeIdentifier`, TLS with Application-Layer Protocol Negotiation
- Answering an HTTP-01 challenge on port 80: [RFC 8555](https://www.rfc-editor.org/rfc/rfc8555), search for: HTTP Challenge, `/.well-known/acme-challenge/`, key authorization
