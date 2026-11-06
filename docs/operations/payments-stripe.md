# Supporter payments with Stripe

With `payments = stripe`, local users subscribe to monthly or yearly plans on Stripe's hosted payment page (Checkout), and the plan's role is granted and removed as the payment state changes.
The deployment keys are described in the [configuration reference](../reference/configuration.md#payments). For the program as a whole (plans, roles, the list of members), see [supporters.md](supporters.md).
An administrator enables the program and creates plans in the administration. This setting only lets the server talk to Stripe.

One server can use only one provider. To switch to another provider later, ask the existing subscribers to subscribe again with the new one.

## What the server does at Stripe

[How a subscription works](supporters.md#how-a-subscription-works) shows in a diagram how the payment page, the webhook, the reconciliation, the plan's role, and the income fit together.

- **Plans and prices**: For each plan the server creates a Product through the API (ID `my_plan_<plan ID>`, with `metadata.my_plan_id`) and one Price per interval and currency (`lookup_key` is `my_<plan ID>_<month|year>_<currency>`).
  A Stripe Price cannot change its amount, so changing the amount creates a new Price and archives the old one. Subscriptions on the old Price renew at its amount. The discount for yearly payment is expressed in the amount of the yearly Price; no coupons are used.
  Do not edit or delete these Products and Prices in the dashboard.
  A plan can use the currencies in [Stripe's list of minimum charges](https://docs.stripe.com/currencies#minimum-and-maximum-charge-amounts), and its monthly and yearly amounts must each reach that minimum: USD, AED, ARS, AUD, BRL, CAD, CHF, COP, CZK, DKK, EUR, GBP, HKD, HUF, IDR, ILS, INR, JPY, KRW, MXN, MYR, NOK, NZD, PHP, PLN, RON, RUB, SEK, SGD, THB, and ZAR. The server refuses any other currency.
  The list holds for a charge in the account's settlement currency. A charge that Stripe converts must meet the settlement currency's minimum after the conversion, and the server does not check that, so choose a plan's amount well above the minimum when its currency differs from your bank account's.
- **Subscribing**: The server creates a Checkout session with `mode=subscription` and sends the user's browser to its URL (`checkout.stripe.com`). The payment page expires after one hour.
  The server's subscription ID goes into `client_reference_id`, `metadata.my_subscription_id`, and the `metadata.my_subscription_id` of the Stripe subscription. Automatic tax calculation (Stripe Tax) is not enabled.
- **Starting again**: At most one page of a pending subscription can take a payment. A user who starts again on the same plan and interval returns to the open page, which opens empty again, unless it has less than 15 minutes left. Any other start gets a new page, and the server expires the earlier one, which then shows Stripe's expired message. If an earlier page was paid already, the user gets no page: they go to the return screen, which shows the subscription. If the server cannot expire an earlier page, the start fails and the user can try again.
  A pending subscription that closes (its plan retired, its account deleted) expires its page in the same way.
- **A second subscription**: Two payments can still meet, when both pages are submitted at the same moment. The subscription that reaches the server first is the user's. The server cancels the other one at once and refunds all its paid invoices in full, renewals included, since it gave nothing for any period, with the reason `duplicate`, and logs `a second subscription was canceled and refunded`. A payment for a pending subscription that has closed is treated the same way, also when it reached the server just before the close. The user receives Stripe's refund and sees one subscription. Stripe does not return its fee on the refunded charge, and that fee shows in the income.
  A charge in a dispute is not refunded, and neither is a payment that had not settled when the server looked (a debit method, for example). A subscription that someone already cancelled in the dashboard is left as it is, with its charges, and it never becomes the user's subscription.
  **Before upgrading a server from a version without this**, look in Stripe for customers with two subscriptions whose `metadata.my_subscription_id` is the same. The first reconciliation cancels the one not in use and refunds everything it charged. Cancel in the dashboard any that you would rather settle by hand, and the server leaves them alone.
- **Customer portal**: On first use, the server creates through the API a portal configuration (`metadata.my=1`) that enables only updating payment methods, the invoice history, and cancellation at the end of the period. Plan changes are made on this server's screens, not in the portal.
  A cancellation in the portal shows on this server as scheduled to cancel, and the user can undo it from the server's screen. A plan change is refused until it is undone.
- **A cancellation date set by hand**: The portal sets the subscription's `cancel_at`, not `cancel_at_period_end`, and the server reads both. A `cancel_at` you set in the dashboard or through the API counts the same when it is not after the end of the current period. When it is before that end, the server shows that date as the end of the subscription, and the user keeps the role until then. A later `cancel_at` is not a cancellation yet: the subscription stays active until the period that holds the date.
- **Plan changes**: An upgrade, and a change from monthly to yearly, bills the difference prorated at once (`proration_behavior=always_invoice`) and switches only when that invoice is paid (`payment_behavior=pending_if_incomplete`). A downgrade, and a change from yearly to monthly, switch at the end of the period through a Subscription Schedule.
- **A failed plan change**: When the payment of a change fails or the card asks for authentication, the server voids that invoice, which discards the change, and the user sees an error. The subscription stays on its plan and active, with no grace period and no failed-payment notification. The user can update the card in the portal and try again, and is charged once. If the payment is still processing, the server leaves it, and the plan switches when it succeeds. Renewals are not affected: a failed renewal goes to the grace period as described below.
- **Reading state back**: The server treats a webhook as a signal and reads the subscription, the Checkout session, and the money movements back from the Stripe API to decide the state.
  In addition, a reconciliation every `payments_reconcile_interval` (default 6 hours) reads the event list, the unfinished Checkout sessions, the balance transactions, and a sweep of all subscriptions, and repairs any webhook the server missed.
- **Income**: The server records the gross amount, the fee, and the net of each balance transaction. Fees such as Billing's 0.7% appear as separate `stripe_fee` transactions and are subtracted from the income.
  `stripe_fee` covers the fees of the whole account, so **use a Stripe account dedicated to this server**. On an account used for other sales, those fees are subtracted from the income too.
- **Dates**: Income dates are counted in the time zone of the Stripe account (the time zone under "Settings > Business > Account details" in the dashboard, `settings.dashboard.timezone` in the API). When it cannot be read, the server counts in UTC and reads it again every hour.

Each server process sends at most 20 requests per second (10 in test mode). [Stripe's limits](https://docs.stripe.com/rate-limits) are per account: 100 requests per second in live mode, 25 in a sandbox, and 25 per second for each endpoint. Processes of one server share them, as does anything else that uses the account.
When Stripe returns `429`, the server waits as long as `Retry-After` says if that is 30 seconds or less (1 second without the header) and retries up to 2 times; beyond that it leaves the retry to the job.

Stripe also allocates read requests per account: an average of 500 per transaction over 30 days, and at least 10,000 a month whatever the number of transactions. At the default interval, reconciliation stays far below that on a small server. A reconciliation that finds nothing still reads a few pages, so the shortest interval of 15 minutes (2,880 runs in 30 days) comes close to the 10,000 on an account with few transactions.
Stripe limits each subscription to 10 new invoices a minute and 20 a day, and a plan change billed at once makes a new invoice.

## Card data and PCI DSS

Payment methods are entered only on Stripe's Checkout and customer portal, and this server's web client loads neither Stripe's script nor an iframe.
The server stores only Stripe object IDs (`cus_`, `sub_`, `price_`, `prod_`, `cs_`, `txn_`), amounts, periods, and states. It does not store card numbers, last four digits, expiry dates, or billing addresses. Webhook bodies and Stripe responses are kept in neither the logs nor the database.
For this reason, the operator's PCI DSS scope is expected to be that of a merchant using a hosted payment page (SAQ A). Check the scope that applies and the type of self-assessment questionnaire in Stripe's dashboard guidance ("Settings > Compliance").

## 1. Prepare the Stripe account

1. Create a Stripe account and enter your business information in the dashboard. If you sell in Japan, register the URL of your commerce disclosure page under "Settings > Business > Public information > Notice based on the Act on Specified Commercial Transactions", and a link appears below Checkout.
2. Check the time zone under "Settings > Business > Account details". Income dates are counted in it.
3. Under "Settings > Billing > Subscriptions and emails > Manage failed payments", choose the retry behavior (Smart Retries) and what happens when retries run out.
   The grace period before the role is removed (7 days from the first failure) is counted by this server, so Stripe's retry settings do not affect it. When retries run out, "Cancel the subscription" is recommended.
4. To send receipts to subscribers, enable the successful-payment receipt under "Settings > Customer emails".

## 2. Create an API key

Under "Developers > API keys", creating a **restricted key** (`rk_test_...` / `rk_live_...`) is recommended. A secret key (`sk_test_...` / `sk_live_...`) works too.
The prefix of the key sets the mode, and a test key charges nothing. A publishable key (`pk_`) cannot be used.

Give a restricted key these permissions. A restricted key with this table's permissions ran subscribing, plan changes, cancellation, reconciliation, and voiding invoices in a sandbox. Refunds, expiring a page, and disputes have not been run with it, and nobody has checked that the table is the smallest set that works.

| Resource | Permission | APIs used |
| --- | --- | --- |
| Products | Write | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | Write | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | Write | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}`, `POST /v1/checkout/sessions/{id}/expire` |
| Customer portal | Write | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | Write | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules (part of Subscriptions) | Write | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | Write | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments`, `POST /v1/invoices/{id}/void` |
| Charges, Refunds | Write | `GET /v1/charges/{id}`, `GET /v1/refunds`, `POST /v1/refunds` |
| Disputes | Read | Read through the balance transactions and charges |
| Balance transaction sources | Read | `GET /v1/balance_transactions` |
| Events | Read | `GET /v1/events` |
| Account (information of an account outside Connect) | Read | `GET /v1/account` (time zone) |

Without the permission to write refunds, the server still cancels a second subscription, and `duplicate_charge` stays in the warnings (see [Register the webhook endpoint](#3-register-the-webhook-endpoint)) until it can refund. A key created before this permission was listed needs it added.

Pass the key in the environment variable `MY_PAYMENTS_STRIPE_SECRET_KEY` (or the path of a file in `MY_PAYMENTS_STRIPE_SECRET_KEY_FILE`). It cannot be written in the configuration file.
If the key leaks, rotate it in the dashboard and redeploy with the new key.

## 3. Register the webhook endpoint

Under "Developers > Webhooks > Add endpoint", register the following. The server does not create the endpoint through the API.

- URL: `https://<server domain>/payments/webhooks/stripe`
- Events from: "Your account" (not Connected accounts)
- Payload style: snapshot. The server reads only `id`, `type`, `livemode`, `created`, `data.object.id`, and `data.object.object` of the body, so the endpoint's API version does not matter.
- Events to send (16 types):

  - `checkout.session.completed`, `checkout.session.expired`
  - `customer.subscription.created`, `customer.subscription.updated`, `customer.subscription.deleted`, `customer.subscription.paused`, `customer.subscription.resumed`
  - `invoice.paid`, `invoice.payment_failed`
  - `charge.refunded`, `refund.created`, `refund.updated`
  - `charge.dispute.created`, `charge.dispute.closed`, `charge.dispute.funds_withdrawn`, `charge.dispute.funds_reinstated`

Pass the endpoint's signing secret (`whsec_...`) in the environment variable `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` (or `_FILE`).

The endpoint verifies the signature (HMAC-SHA256 in `Stripe-Signature`) and the timestamp (within 5 minutes either way), and rejects a test event received with a live key (and the reverse) with `400`.
It records each event once by ID, enqueues a job, and returns `2xx`. When it cannot write to the database it returns `503`, and Stripe sends the event again. The body is limited to 1 MiB.
While you rotate the signing secret, Stripe signs with both the old and the new one, so putting the new secret in the configuration is enough.

The state of the provider (the mode, when the last webhook arrived, and the result and failure kind of the last reconciliation) is shown in the "Payment provider" section of `/admin/supporters` in the administration, and by the admin API `GET /api/v1/admin/payment_status` (permission `manage_supporters`). The section explains how to fix each kind of reconciliation failure.
`warnings` lists `no_webhook` (no webhook has ever been recorded) and `reconciliation_corrected` (the last reconciliation repaired a subscription). Suspect a misconfigured webhook when either appears.
It also lists `duplicate_charge`: a second subscription is not cancelled and refunded yet. See [Operation](#7-operation).
"Reconcile now" in the section (`POST /api/v1/admin/payment_reconciliation/runs`) reconciles at once instead of waiting for the scheduled run. It is limited to once per UTC hour (a second call in the same hour returns `429`), and the audit log records `payment_reconcile_queued`.

## 4. Configure the server

```sh
MY_PAYMENTS=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifies the configuration. A binary built with the build tag `no_payments_stripe` refuses `payments = stripe` (see [build-tags.md](../reference/build-tags.md)).

## 5. Try it in test mode

Start with a test key (`rk_test_` / `sk_test_`, or a sandbox key). Test mode charges nothing.
Test keys and live keys have separate Products, Prices, customers, subscriptions, and webhook endpoints. To go live, register the endpoint again in live mode, replace the key and the signing secret with the live ones, and redeploy. The plans' Prices are created anew in the live account.

Test cards ([Stripe documentation](https://docs.stripe.com/testing)):

| Number | Behavior |
| --- | --- |
| `4242 4242 4242 4242` | Succeeds |
| `4000 0000 0000 0341` | Can be attached but the charge fails (to check payment failure and the start of the grace period) |
| `4000 0000 0000 9995` | Fails for insufficient funds |
| `4000 0000 0000 0259` | A dispute is created after the payment |

Use any future expiry date and any 3-digit security code.

To try renewal and payment failure, use a [test clock](https://docs.stripe.com/billing/testing/test-clocks/api-advanced-usage). A customer created by Checkout has no clock at creation, so subscribe first and then create a clock for that customer through the API, passing `customer` to `POST /v1/test_helpers/test_clocks` (API version `2026-05-27.dahlia` or later).
Stripe allows this only when `frozen_time` is not in the past, the account has no Automations configured, and the customer has at most three subscriptions. The customer cannot leave the clock afterwards, and deleting the clock deletes the customer and its subscriptions. Advance time under "Billing > Test clocks" in the dashboard to trigger the renewal charge and the failure.

A clock cannot show everything:

- **The end of the grace period**: The server compares real time with the time Stripe reports for the failure (`finalized_at`), which is simulated and ahead of real time. The subscription enters the grace period, and advancing the clock further does not end it. The role stays until real time passes 7 days after the simulated failure, or until Stripe cancels the subscription when its retries run out, if the account is set to do that.
- **The sweep of all subscriptions**: Stripe's subscription list leaves out subscriptions on a clock unless the request names the clock or the customer, and the server's sweep names neither. Only webhooks and the event list bring their changes to the server.

## 6. Try it locally (Stripe CLI)

The [Stripe CLI](https://docs.stripe.com/stripe-cli) (Apache-2.0) forwards Stripe events to a server on your machine. No public URL is needed.

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

Put the `whsec_...` that `stripe listen` prints in `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` and start the server (match the port to your `listen` setting).

With Docker Compose, set `MY_PAYMENTS=stripe`, `MY_PAYMENTS_STRIPE_SECRET_KEY`, and `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` in `deploy/docker/.env`, then run `docker compose up -d`.

In another terminal, you can fire single events.

```sh
stripe trigger checkout.session.completed
stripe trigger invoice.payment_failed
stripe trigger charge.refunded
stripe trigger charge.dispute.created
stripe events resend evt_...   # send an event you missed again (up to 30 days)
```

The objects that `stripe trigger` creates carry no subscription ID of this server, so no subscription changes state (the server reads them back and ignores what is not its own subscription). To check the whole subscription flow, subscribe from the web client and pay with a test card.

## 7. Operation

- A reconciliation failure of kind `permission` means a wrong key or missing permissions. The server's log line (`payment reconciliation failed`) has Stripe's message, which names the permission to add.
- When reconciliations often hit the request limit, raise `payments_reconcile_max_requests` or lengthen `payments_reconcile_interval`. Every read counts toward Stripe's read allocation (see [What the server does at Stripe](#what-the-server-does-at-stripe)), so prefer the longer interval on an account with few transactions.
- When reconciliation has been stopped for more than 30 days, the server cannot catch up from Stripe's event list (which reaches back 30 days). The sweep of all subscriptions repairs the state then.
- `duplicate_charge` in the warnings means that a user paid twice and the server could not finish cancelling and refunding the second subscription. The log line `a second subscription could not be canceled and refunded` names both subscriptions and the kind of failure, and appears again at each reconciliation, which tries again. With kind `permission`, add the permission to write refunds. Otherwise refund the charge in the dashboard: the next try finds nothing left to refund and clears the warning. "Reconcile now" tries at once.
- Other refunds and chargebacks are handled in Stripe's dashboard. A full refund and a chargeback remove the role and cancel the subscription at once, and a partial refund does not remove the role. Winning a dispute does not restore the role automatically.

## Items to verify

These have not been tried against Stripe, in a sandbox or live, and rest on Stripe's documentation.

- A renewal of a subscription on an archived Price, which should stay on that Price.
- `status_transitions.finalized_at` at a real renewal. The server takes it as the time the payment first failed, and counts the grace period from it.
- Whether a `stripe_fee` transaction can be assigned to a subscription. The server does not try: it subtracts these fees from the income as a whole.
- Whether `settings.dashboard.timezone` of `GET /v1/account` is read with a restricted key. A failed read does not stop the server, which counts in UTC, so a working server does not show it. Check that income dates fall in the account's time zone.
- That `GET /v1/invoice_payments` filters by `payment[payment_intent]`, which links a payment to its subscription.
- Full and partial refunds and chargebacks, from the dashboard to the state on the server.
- That the sweep of all subscriptions repairs a subscription whose state the server missed.
- What an upgrade does for a subscription paying with a method that pending updates do not list (ACH, SEPA, and Bacs direct debits). Keep those methods off in the dashboard until this is confirmed.
- That `cancel_at_period_end=true` replaces a `cancel_at` in a later period.
- Returning to an open page. The server compares what `GET /v1/checkout/sessions/{id}` returns (`line_items`, `success_url`, `cancel_url`, `customer`, `customer_email`) with the new request, and any difference makes a new page. If Stripe rewrites `success_url` or `customer_email` when it stores them, the user gets a new page each time and loses what they typed, but pays once.
- Expiring a session as its payment completes. Stripe's documentation says that a session that is not open returns an error, and the server then reads the session. The error's exact code and the status while a card authentication is in progress are not documented.
- That one session cannot be paid twice from two tabs. A session holds one subscription and completes once, but the documentation does not state it for two submissions at the same moment.
- Whether a failed authentication on the payment page leaves an `incomplete` subscription that names the server's subscription. The server would link it and keep the user pending, and a payment on another page would be refunded as the second one.
- A refund of a second subscription, with a restricted key whose "Charges" and "Refunds" permissions are as the table says, and the money movements that follow it.
