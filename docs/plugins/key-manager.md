<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Key manager protocol

The services, messages, and enums of the package `my.plugin.key_manager.v1`, which [`key_manager.proto`](../../api/plugin/my/plugin/key_manager/v1/key_manager.proto) defines.

Keeping the key-encryption key (KEK) of the server's secrets in a service of the operator's choice, such as Vault or an
HSM. The requests hold data-encryption keys in plaintext and the responses open them, so a plugin must never log dek, aad,
or wrapped, and the server refuses plain TCP to this kind. Whoever runs the plugin can read every secret of the server.

A plugin answers with an outcome, and fails with an HTTP or a Connect error when it cannot answer. The server does not
fall back on anything: a failure fails the request or the job that needed the key, and the job retries.

## Services

### `KeyManagerService`

#### `Describe`

```proto
rpc Describe(DescribeRequest) returns (DescribeResponse);
```

Request: [`DescribeRequest`](#describerequest). Response: [`DescribeResponse`](#describeresponse).

Describe returns the ID of the KEK that WrapKey uses now. The server calls it at each start, and a failure stops the
start. It has no side effect.

#### `WrapKey`

```proto
rpc WrapKey(WrapKeyRequest) returns (WrapKeyResponse);
```

Request: [`WrapKeyRequest`](#wrapkeyrequest). Response: [`WrapKeyResponse`](#wrapkeyresponse).

WrapKey encrypts a data-encryption key under the current KEK, authenticating aad. Wrapping the same key twice must give
different values: use a fresh nonce. The server keeps what comes back beside the secret, for as long as the secret lasts.

#### `UnwrapKey`

```proto
rpc UnwrapKey(UnwrapKeyRequest) returns (UnwrapKeyResponse);
```

Request: [`UnwrapKeyRequest`](#unwrapkeyrequest). Response: [`UnwrapKeyResponse`](#unwrapkeyresponse).

UnwrapKey opens what WrapKey returned. It must refuse a changed wrapped value and a different aad with OUTCOME_DECRYPT:
that is what keeps a secret from being copied to another row. The server tests this at each start, with a key it throws
away, and refuses to start on a plugin that does not.

## Messages

### `DescribeRequest`

No fields.

### `DescribeResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.
- `key_id` (`string`, field 2): With OUTCOME_OK, the ID of the current KEK: 1 to 248 bytes of visible ASCII. It is no secret: the server stores it in
  plain text beside each wrapped key, and names it in logs and in the output of commands.

### `WrapKeyRequest`

- `dek` (`bytes`, field 1): The data-encryption key in plaintext, 32 bytes.
- `aad` (`bytes`, field 2): The additional authenticated data, which ties the key to where the secret is stored. It holds row IDs and column names.

### `WrapKeyResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure, and the server stores nothing.
- `key_id` (`string`, field 2): With OUTCOME_OK, the ID of the KEK that wrapped the key, in the form of DescribeResponse.key_id. A KEK that rotated shows
  here: the server takes it as the current one.
- `wrapped` (`bytes`, field 3): With OUTCOME_OK, the wrapped key: 1 to 65,535 bytes, in which the plaintext of the key does not appear.

### `UnwrapKeyRequest`

- `key_id` (`string`, field 1): The ID of the KEK that wrapped the key, as WrapKey returned it. It may be an ID older than the current one.
- `wrapped` (`bytes`, field 2): The wrapped key, as WrapKey returned it.
- `aad` (`bytes`, field 3): The additional authenticated data of the WrapKey call.

### `UnwrapKeyResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but a value below, an empty response included, is a failure.
- `dek` (`bytes`, field 2): With OUTCOME_OK, the data-encryption key in plaintext.

## Enums

### `Outcome`

The result of a call. The server treats a value it does not know as a failure. Connect errors and HTTP errors say only
that the call failed in transport, which is a failure too: a plugin must not answer OUTCOME_DECRYPT or OUTCOME_UNKNOWN_KEY
when its key store is out of reach.

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_OK` = 1: The call succeeded, and the response holds its result.
- `OUTCOME_UNKNOWN_KEY` = 2: UnwrapKey: the plugin does not hold the KEK of key_id.
- `OUTCOME_DECRYPT` = 3: UnwrapKey: wrapped or aad does not authenticate.
