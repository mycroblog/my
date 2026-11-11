# Web Push sender

- Changing the request to a push service or the handling of its answers: [RFC 8030](https://www.rfc-editor.org/rfc/rfc8030), search for: `TTL`, `Urgency`, `Topic`, 410 (Gone), Requesting Push Message Delivery
- Changing the encryption of a message: [RFC 8291](https://www.rfc-editor.org/rfc/rfc8291), search for: `aes128gcm`, `ua_public`, `auth_secret`, Key Derivation, `WebPush: info`
- Changing the record of the encrypted body (header, padding, record size): [RFC 8188](https://www.rfc-editor.org/rfc/rfc8188), search for: `aes128gcm`, `rs`, `keyid`, padding delimiter
- Changing the VAPID authorization: [RFC 8292](https://www.rfc-editor.org/rfc/rfc8292), search for: `vapid`, `aud`, `exp`, `sub`, `k`
