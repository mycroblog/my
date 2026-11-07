<!-- Code generated from api/plugin by "scripts/task.sh api:plugin-generate". DO NOT EDIT. -->

# Billing protocol

The services, messages, and enums of the package `my.plugin.billing.v1`, which [`billing.proto`](../../api/plugin/my/plugin/billing/v1/billing.proto) defines.

The costs of one or more providers that the server has no adapter of. A plugin is one source of the server: it
aggregates its providers itself, and tells them apart by the service of a line.

## Services

### `BillingService`

#### `Fetch`

```proto
rpc Fetch(FetchRequest) returns (FetchResponse);
```

Request: [`FetchRequest`](#fetchrequest). Response: [`FetchResponse`](#fetchresponse).

Fetch returns the costs of the months as the providers report them now. The server calls it about once a day, and
again when a call fails with OUTCOME_THROTTLED, OUTCOME_UNAVAILABLE, or a transport error, so the same call may come
twice. It lasts plugin_billing_timeout at most.

## Messages

### `FetchRequest`

- `months` (repeated `string`, field 1): The billing months to report, YYYY-MM, oldest first, one or two. The plugin_billing_location setting of the server
  names the time zone that cuts them.
- `max_requests` (`int32`, field 2): How many billed requests the plugin may make to its providers. 0 forbids any: answer OUTCOME_REQUEST_CAP. 2147483647
  means no cap.

### `FetchResponse`

- `outcome` ([`Outcome`](#outcome), field 1): A name in JSON. Anything but OUTCOME_OK, an empty response included, is a failure that records no cost: the imported
  costs of the server stay as they were.
- `lines` (repeated [`CostLine`](#costline), field 2): One line per month, service, and charge type. With OUTCOME_OK, a line that an earlier call returned and this one does
  not has no cost any more. The server ignores the lines of any other outcome.
- `requests` (`int32`, field 3): The billed requests made, which the server records even when the outcome is a failure. Never above max_requests.
- `bytes_billed` (`int64`, field 4): The bytes billed, if the providers bill by them, as a decimal string in JSON.

### `CostLine`

The cost of a billing month, service, and charge type, with its offsetting lines, credits, and refunds summed.

- `month` (`string`, field 1): The billing month, YYYY-MM: one of the months of the request.
- `covered_start` (`string`, field 2): The first day the amount covers, YYYY-MM-DD, inside the month.
- `covered_end` (`string`, field 3): The last day the amount covers, YYYY-MM-DD, inside the month: its last day once final, the last day with data before.
- `service` (`string`, field 4): The name of the service, at most 200 characters, without control characters. It may name the provider too.
- `charge_type` ([`ChargeType`](#chargetype), field 5): A name in JSON. Anything but a value below, an empty line included, makes the response invalid.
- `category` ([`Category`](#category), field 6): A name in JSON. Anything but a value below, an empty line included, makes the response invalid.
- `amount` (`string`, field 7): A decimal in major units, unrounded, such as "-1.75".
- `currency` (`string`, field 8): An ISO 4217 code.
- `final` (`bool`, field 9): The provider has closed the month.

## Enums

### `Outcome`

The result of a call. Connect errors say only that the call failed in transport, which the server retries.

- `OUTCOME_UNSPECIFIED` = 0
- `OUTCOME_OK` = 1
- `OUTCOME_PERMISSION` = 2: The credentials may not read the billing data. The next daily import tries again, as with every outcome that does
  not say the server retries.
- `OUTCOME_NOT_ENABLED` = 3: The provider's billing data is not enabled.
- `OUTCOME_NOT_FOUND` = 4: The billing data does not exist.
- `OUTCOME_THROTTLED` = 5: The provider throttles the requests. The server retries.
- `OUTCOME_REQUEST_CAP` = 6: The call would make more billed requests than max_requests.
- `OUTCOME_BYTES_CAP` = 7: The call would scan more bytes than the plugin allows itself.
- `OUTCOME_INVALID_RESPONSE` = 8: The provider's answer is not usable.
- `OUTCOME_UNAVAILABLE` = 9: The provider did not answer. The server retries.

### `ChargeType`

- `CHARGE_TYPE_UNSPECIFIED` = 0
- `CHARGE_TYPE_USAGE` = 1
- `CHARGE_TYPE_CREDIT` = 2
- `CHARGE_TYPE_REFUND` = 3
- `CHARGE_TYPE_TAX` = 4
- `CHARGE_TYPE_SUPPORT` = 5
- `CHARGE_TYPE_FEE` = 6
- `CHARGE_TYPE_ADJUSTMENT` = 7
- `CHARGE_TYPE_OTHER` = 8

### `Category`

- `CATEGORY_UNSPECIFIED` = 0
- `CATEGORY_COMPUTE` = 1
- `CATEGORY_DATABASE` = 2
- `CATEGORY_STORAGE` = 3
- `CATEGORY_NETWORK` = 4
- `CATEGORY_DOMAIN` = 5
- `CATEGORY_EMAIL` = 6
- `CATEGORY_SOFTWARE` = 7
- `CATEGORY_OTHER` = 8
