# Supporter payments with Stripe

A guide for operators. The deployment keys are described in the [configuration reference](../reference/configuration.md#payments_provider). For the program as a whole (plans, roles, the list of members), see [supporters.md](supporters.md).

With `payments_provider = stripe`, local users subscribe to monthly or yearly plans on Stripe's hosted payment page (Checkout), and the plan's role is granted and removed as the payment state changes.
An administrator enables the program and creates plans in the administration. This setting only lets the server talk to Stripe.

One server can use only one provider. To switch to another provider later, ask the existing subscribers to subscribe again with the new one.

## What the server does at Stripe

- **Plans and prices**: For each plan the server creates a Product through the API (ID `my_plan_<plan ID>`, with `metadata.my_plan_id`) and one Price per interval and currency (`lookup_key` is `my_<plan ID>_<month|year>_<currency>`).
  A Stripe Price cannot change its amount, so changing the amount creates a new Price and archives the old one. Subscriptions on the old Price renew at its amount. The discount for yearly payment is expressed in the amount of the yearly Price; no coupons are used.
  Do not edit or delete these Products and Prices in the dashboard.
- **Subscribing**: The server creates a Checkout session with `mode=subscription` and sends the user's browser to its URL (`checkout.stripe.com`). The payment page expires after one hour.
  The server's subscription ID goes into `client_reference_id`, `metadata.my_subscription_id`, and the `metadata.my_subscription_id` of the Stripe subscription. Automatic tax calculation (Stripe Tax) is not enabled.
- **Customer portal**: On first use, the server creates through the API a portal configuration (`metadata.my=1`) that enables only updating payment methods, the invoice history, and cancellation at the end of the period. Plan changes are made on this server's screens, not in the portal.
- **Plan changes**: An upgrade switches at once and bills the difference prorated (`proration_behavior=always_invoice`). A downgrade, and a change from yearly to monthly, switch at the end of the period through a Subscription Schedule.
- **Reading state back**: The server treats a webhook as a signal and reads the subscription, the Checkout session, and the money movements back from the Stripe API to decide the state.
  In addition, a reconciliation every `payments_reconcile_interval` (default 6 hours) reads the event list, the unfinished Checkout sessions, the balance transactions, and a sweep of all subscriptions, and repairs any webhook the server missed.
- **Income**: The server records the gross amount, the fee, and the net of each balance transaction. Fees such as Billing's 0.7% appear as separate `stripe_fee` transactions and are subtracted from the income.
  `stripe_fee` covers the fees of the whole account, so **use a Stripe account dedicated to this server**. On an account used for other sales, those fees are subtracted from the income too.
- **Dates**: Income dates are counted in the time zone of the Stripe account (the time zone under "Settings > Business > Account details" in the dashboard, `settings.dashboard.timezone` in the API). When it cannot be read, the server counts in UTC and reads it again every hour.

The server sends at most 20 requests per second (10 in test mode). When Stripe returns `429`, the server waits as long as `Retry-After` says if that is 30 seconds or less and retries up to 2 times; beyond that it leaves the retry to the job.

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

Give a restricted key these permissions (**not yet verified**: the exact names and any missing or excess permissions have to be checked in test mode).

| Resource | Permission | APIs used |
| --- | --- | --- |
| Products | Write | `GET`, `POST /v1/products`, `POST /v1/products/{id}` |
| Prices | Write | `GET`, `POST /v1/prices`, `POST /v1/prices/{id}` |
| Checkout Sessions | Write | `POST /v1/checkout/sessions`, `GET /v1/checkout/sessions/{id}` |
| Customer portal | Write | `GET`, `POST /v1/billing_portal/configurations`, `POST /v1/billing_portal/sessions` |
| Subscriptions | Write | `GET /v1/subscriptions`, `POST`, `DELETE /v1/subscriptions/{id}` |
| Subscription Schedules (part of Subscriptions) | Write | `POST /v1/subscription_schedules`, `POST /v1/subscription_schedules/{id}`, `POST /v1/subscription_schedules/{id}/release` |
| Invoices | Read | `GET /v1/invoices/{id}`, `GET /v1/invoice_payments` |
| Charges, Refunds, Disputes | Read | `GET /v1/charges/{id}`, `GET /v1/refunds` |
| Balance transactions (Balance) | Read | `GET /v1/balance_transactions` |
| Events | Read | `GET /v1/events` |
| Account (information of an account outside Connect) | Read | `GET /v1/account` (time zone) |

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
`warnings` lists `no_webhook` (no webhook has ever been recorded) and `reconcile_corrected` (the last reconciliation repaired a subscription). Suspect a misconfigured webhook when either appears.
"Reconcile now" in the section (`POST /api/v1/admin/payment_reconciles`) reconciles at once instead of waiting for the scheduled run. It is limited to once per UTC hour (a second call in the same hour returns `429`), and the audit log records `payment_reconcile_queued`.

## 4. Configure the server

```sh
MY_PAYMENTS_PROVIDER=stripe
MY_PAYMENTS_STRIPE_SECRET_KEY=rk_test_...
MY_PAYMENTS_STRIPE_WEBHOOK_SECRET=whsec_...
# Optional: the reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=6h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifies the configuration. A binary built with the build tag `no_payments_stripe` refuses `payments_provider = stripe` (see [build-tags.md](../reference/build-tags.md)).

## 5. Try it in test mode

Start with a test key (`rk_test_` / `sk_test_`, or a sandbox key). Test mode charges nothing.
Test keys and live keys have separate Products, Prices, customers, subscriptions, and webhook endpoints. To go live, register the endpoint again in live mode, replace the key and the signing secret with the live ones, and redeploy. The plans' Prices are created anew in the live account.

Test cards ([Stripe documentation](https://docs.stripe.com/testing)):

| Number | Behavior |
| --- | --- |
| `4242 4242 4242 4242` | Succeeds |
| `4000 0000 0000 0341` | Can be attached but the charge fails (to check payment failure and the grace period) |
| `4000 0000 0000 9995` | Fails for insufficient funds |
| `4000 0000 0000 0259` | A dispute is created after the payment |

Use any future expiry date and any 3-digit security code.

To try renewal, payment failure, and the grace period, use a test clock. Subscribe with a customer attached to a test clock and advance time under "Billing > Test clocks" in the dashboard to trigger the renewal charge and the failure.

## 6. Try it locally (Stripe CLI)

The [Stripe CLI](https://docs.stripe.com/stripe-cli) (Apache-2.0) forwards Stripe events to a server on your machine. No public URL is needed.

```sh
stripe login
stripe listen --forward-to http://localhost:8080/payments/webhooks/stripe \
  --events checkout.session.completed,checkout.session.expired,customer.subscription.created,customer.subscription.updated,customer.subscription.deleted,customer.subscription.paused,customer.subscription.resumed,invoice.paid,invoice.payment_failed,charge.refunded,refund.created,refund.updated,charge.dispute.created,charge.dispute.closed,charge.dispute.funds_withdrawn,charge.dispute.funds_reinstated
```

Put the `whsec_...` that `stripe listen` prints in `MY_PAYMENTS_STRIPE_WEBHOOK_SECRET` and start the server (match the port to your `listen` setting).
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

- A reconciliation failure of kind `permission` means a wrong key or missing permissions.
- When reconciliations often hit the request limit, raise `payments_reconcile_max_requests` or lengthen `payments_reconcile_interval`.
- When reconciliation has been stopped for more than 30 days, the server cannot catch up from Stripe's event list (which reaches back 30 days). The sweep of all subscriptions repairs the state then.
- Refunds and chargebacks are handled in Stripe's dashboard. A full refund and a chargeback remove the role and cancel the subscription at once, and a partial refund does not remove the role. Winning a dispute does not restore the role automatically.

## Items to verify

The following behaviors come from Stripe's documentation and are to be confirmed in test mode.

- Changing the price at the end of the period with a Subscription Schedule, and releasing that schedule.
- That a subscription on an archived Price renews on that Price.
- That `latest_invoice` and `schedule` can be expanded in the subscription list, and that the time a payment failed can be taken as the time the invoice was finalized (`status_transitions.finalized_at`).
- That `settings.dashboard.timezone` of `GET /v1/account` can be read with a restricted key.
- The list of permissions a restricted key needs (the table in section 2).
- The table of minimum charge amounts per currency, and the currencies that Stripe treats with different decimal places from ISO 4217 (ISK, HUF, TWD, UGX, and others).
- The exact values of the read rate limits.
