# Fake Web Push service for tests

- Changing what the fake accepts, stores, or answers as a push service: [RFC 8030](https://www.rfc-editor.org/rfc/rfc8030), search for: Receiving Push Messages, `TTL`, `Urgency`, 201 (Created)
- Changing how the fake checks a VAPID authorization: [RFC 8292](https://www.rfc-editor.org/rfc/rfc8292), search for: `vapid`, `aud`, `exp`
- Changing how the fake decrypts a message: [RFC 8291](https://www.rfc-editor.org/rfc/rfc8291), search for: `aes128gcm`, Key Derivation, authentication secret
