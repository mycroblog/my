# Web Push

An app can have the server push the user's notifications to a device with standard Web Push
([RFC 8030](https://www.rfc-editor.org/rfc/rfc8030)), while no stream is open. The instance sends Web Push when
`features` of `GET /api/v1/instance` lists `web_push`; subscribe with its `push_server_key` (the VAPID public key,
[RFC 8292](https://www.rfc-editor.org/rfc/rfc8292)) and register the subscription with
`PUT /api/v1/push/subscription` (scope `push`), naming the notification types in `alerts`. Each grant has one
subscription, which ends when the grant ends or the push service reports it gone.

## The message

The server sends one message for each new notification of a type the subscription takes. Its body is encrypted with
the `aes128gcm` content coding ([RFC 8291](https://www.rfc-editor.org/rfc/rfc8291)) in a single record, and the
plaintext is a JSON object of at most 3993 bytes:

```json
{"notification_id": "0193b0c2-…", "type": "mention", "title": "Alice mentioned you", "lang": "en", "path": "/notifications/0193b0c2-…"}
```

| Field | Meaning |
| --- | --- |
| `notification_id` | The ID of the notification. Fetch it with `GET /api/v1/notifications/{id}` for what it is about. A message sent again carries the same ID, so use it to replace a notification already shown. |
| `type` | The notification's type, as `Notification.type` gives it. |
| `title` | The notification worded for the user, in the user's language, ready to show. |
| `lang` | The language tag of `title`. |
| `path` | The page of the web client that opens the notification. Other clients should not rely on it; use `notification_id`. |

Fields can be added; ignore the ones you do not know.

## Headers

| Header | Value |
| --- | --- |
| `Content-Encoding` | `aes128gcm` |
| `TTL` | `172800`: the push service keeps the message for 48 hours for an offline device. |
| `Urgency` | `normal` |
| `Topic` | The notification's ID without its hyphens (32 hexadecimal digits), so that a message sent again replaces an undelivered one. |

The [stability](stability.md#web-push-messages) page says which parts of the message stay.
