<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Probe protocol

The services, messages, and enums of the package `my.plugin.probe.v1`, which [`probe.proto`](../../api/plugin/my/plugin/probe/v1/probe.proto) defines.

The smallest kind: the server delegates nothing to it. A plugin of any kind may serve it to check its transport,
protocol, and authentication with `my plugin test --kind probe`, before it implements its own kind.

## Services

### `ProbeService`

#### `Echo`

```proto
rpc Echo(EchoRequest) returns (EchoResponse);
```

Request: [`EchoRequest`](#echorequest). Response: [`EchoResponse`](#echoresponse).

Echo returns what it was given. The call has no side effects.

## Messages

### `EchoRequest`

- `payload` (`bytes`, field 1): Base64 in JSON. May be empty.
- `number` (`int64`, field 2): A decimal string in JSON. May be zero.

### `EchoResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure.
- `payload` (`bytes`, field 2): The payload of the request.
- `number` (`int64`, field 3): The number of the request.

## Enums

### `Outcome`

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_OK` = 1
