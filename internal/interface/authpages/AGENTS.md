# Sign-in, consent, and unsubscribe pages

- Changing the authorization request, its redirect, or its errors: [RFC 6749](https://www.rfc-editor.org/rfc/rfc6749), search for: Authorization Request, `response_type`, `state`, `access_denied`, Error Response
- Changing the `iss` parameter of the authorization response: [RFC 9207](https://www.rfc-editor.org/rfc/rfc9207), search for: `iss`, `authorization_response_iss_parameter_supported`
- Changing how a redirect URI of a native app matches (loopback, private-use scheme): [RFC 8252](https://www.rfc-editor.org/rfc/rfc8252), search for: Loopback Interface Redirection, Private-Use URI Scheme Redirection
- Changing the redirect or the open-redirector protections: [RFC 9700](https://www.rfc-editor.org/rfc/rfc9700), search for: Open Redirection, Insufficient Redirection URI Validation
- Changing the one-click unsubscribe endpoint: [RFC 8058](https://www.rfc-editor.org/rfc/rfc8058), search for: `List-Unsubscribe-Post`, `List-Unsubscribe=One-Click`
