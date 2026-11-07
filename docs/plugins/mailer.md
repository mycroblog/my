<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Mailer protocol

The services, messages, and enums of the package `my.plugin.mailer.v1`, which [`mailer.proto`](../../api/plugin/my/plugin/mailer/v1/mailer.proto) defines.

Sending the server's email through a service of the operator's choice, such as a mail provider that offers only an HTTP
API. A message can carry a secret, such as a verification link, so a plugin must not log it, and the server refuses
plain TCP to this kind.

## Services

### `MailerService`

#### `Send`

```proto
rpc Send(SendRequest) returns (SendResponse);
```

Request: [`SendRequest`](#sendrequest). Response: [`SendResponse`](#sendresponse).

Send sends one message to one recipient. The server calls it from background jobs and retries a failure, so the same
message may come twice. It lasts plugin_mailer_timeout at most.

The request holds the parts of the message, not its encoded form. The plugin adds what the server leaves to whoever
sends: a Message-ID header (unique per message, on the domain of envelope_from), a Date header, and the header
Auto-Submitted: auto-generated, which keeps automatic replies from looping.

## Messages

### `SendRequest`

- `category` ([`Category`](#category), field 1): A name in JSON. A value that a plugin does not know is sent like any other.
- `to` (`string`, field 2): The recipient, one bare address (local@domain) in UTF-8. A plugin whose service needs ASCII converts the domain to
  Punycode, and answers OUTCOME_INVALID for a local part it cannot send.
- `envelope_from` (`string`, field 3): The sender, one bare address: the mail_from setting of the server.
- `from_name` (`string`, field 4): The display name of the sender, one line; empty sends the bare address.
- `reply_to` (`string`, field 5): The address that replies go to, one bare address; empty adds no Reply-To.
- `subject` (`string`, field 6): The subject, one line of UTF-8.
- `text` (`string`, field 7): The plain-text body in UTF-8, with "\n" line ends.
- `html` (`string`, field 8): The HTML body in UTF-8; empty sends the plain text only.
- `headers` (map of `string` to `string`, field 9): Headers to add by their canonical name. The server sends only List-Unsubscribe and List-Unsubscribe-Post, one line each.

### `SendResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but a value below, an empty response included, is a temporary failure: the server sends the
  message again later.
- `enhanced_status` (`string`, field 2): The enhanced status code (RFC 3463) of a refusal, such as 5.1.1, if the service names one. The server keeps a code of
  class 5 of a rejection and of class 4 of OUTCOME_TEMPORARY, and ignores anything else: it takes no text of the service.

## Enums

### `Category`

The kind of a message.

- `CATEGORY_UNSPECIFIED` = 0
- `CATEGORY_TRANSACTIONAL` = 1: A verification, a password reset, or another message that nobody can unsubscribe from.
- `CATEGORY_NOTIFICATION` = 2
- `CATEGORY_ANNOUNCEMENT` = 3

### `Outcome`

The result of a call. Connect errors say only that the call failed in transport, which the server retries like
OUTCOME_TEMPORARY.

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_SENT` = 1: The service took the message.
- `OUTCOME_INVALID` = 2: The message cannot be sent as it is, such as a recipient that is not an address. The server does not send it again.
- `OUTCOME_RECIPIENT_REJECTED` = 3: The address does not exist. The server does not send it again, and stops emailing the address.
- `OUTCOME_REJECTED` = 4: The service refused the message for good, for another reason. The server does not send it again.
- `OUTCOME_TEMPORARY` = 5: The service refused the message for now, or did not answer. The server sends it again later.
