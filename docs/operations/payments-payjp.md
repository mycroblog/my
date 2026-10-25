# Supporter payments with PAY.JP

A guide for operators. The deployment keys are described in the [configuration reference](../reference/configuration.md#payments_provider). For the program as a whole (plans, roles, the list of members), see [supporters.md](supporters.md).

With `payments_provider = payjp`, local users subscribe to monthly or yearly plans on this server's payment page, and the plan's role is granted and removed as the payment state changes.
An administrator enables the program and creates plans in the administration. This setting only lets the server talk to PAY.JP.

One server can use only one provider. To switch from Stripe to PAY.JP (or the reverse), ask the existing subscribers to subscribe again with the new provider.

## What the server does at PAY.JP

PAY.JP has API v1 and v2. v2 has no recurring-billing API yet, so the server uses the recurring billing of **API v1** (Plan and Subscription). v1 has neither a hosted payment page nor a customer portal, so this server serves pages in their place.

- **Plans**: For each price the server creates a Plan whose ID is `my_<plan ID>_<month|year>_<amount>` (`metadata.my_plan_id`). A PAY.JP Plan cannot change its amount, so changing the amount creates a new Plan, and the old Plan is not deleted but gets `metadata.my_archived=1`. Subscriptions on the old Plan renew at its amount.
  Do not edit or delete these Plans in the administration of PAY.JP. The only currency is the Japanese yen, and amounts are 50 to 9,999,999 yen.
- **Subscribing**: The server sends the user's browser to its own payment page (`/payments/pages/payjp/checkout`). The page shows the card fields of payjp.js (PAY.JP's iframe) and runs 3-D Secure authentication when it creates the token (the iframe type).
  The server accepts only a token whose 3-D Secure has completed (`three_d_secure_status` is `verified` or `attempted`), adds the card to the customer as the default card (for the first time with the ID `my_<the server's subscription ID>`), and creates a subscription with the server's subscription ID in `metadata.my_subscription_id`. The first charge happens when the subscription is created.
  The page URL is short-lived and carries a state signed by the server (valid for 1 to 2 hours from creation).
- **The card and payment history page** (in place of Stripe's customer portal): From their subscription screen, a subscriber opens `/payments/pages/payjp/portal` to change the card and check recent payments (the URL expires in one hour).
  Changing the card resumes a subscription stopped by a failed payment and charges at once. Cancelling and changing plans are done on this server's subscription screen.
- **Payment failure**: PAY.JP pauses (`paused`) a subscription whose renewal charge failed, and **does not retry**. The server treats the pause as a late payment and keeps the role for a grace period of 7 days from the time of the pause. If the subscriber changes the card in that time, the subscription resumes; if not, the role is removed after 7 days (on the PAY.JP side the subscription stays paused and is not charged).
- **Plan changes**: An upgrade charges the difference for the rest of the current period, prorated, as a one-time charge (no charge under 50 yen), replaces the plan, and charges the new plan's full amount at the end of the period (the start of the period does not move). If the charge for the difference is declined, the plan does not change.
  A downgrade, and a change from yearly to monthly, switch at the end of the period with `next_cycle_plan`.
- **Reading state back**: The server treats a webhook as a signal and reads the subscription and the money movements back from the PAY.JP API to decide the state.
  In addition, a reconciliation every `payments_reconcile_interval` reads the event list, the money movements, and a sweep of all subscriptions, and repairs any webhook the server missed.
- **Income**: For each payment the server records the gross amount, the fee (computed from the charge's `fee_rate`, rounding down below one yen), and the net. A refund and a chargeback record the returned fee as a negative fee.
  When a term closes and a statement is issued, the difference between the statement's fee and the server's computation is recorded as a fee movement (its ID is the statement's ID), so that the term's fee total matches the statement.
  A statement covers the fees of the whole account, so **use a PAY.JP account dedicated to this server**.
- **Dates**: Income dates are counted in Japan time.

The server sends at most 20 requests per second (2 in test mode). That stays within PAY.JP's rate limit (for secret-key requests, 30 per second in live mode and 2 in test mode). On `429` the server waits 1 second, doubling each time, and retries up to 3 times.
In test mode a single reconciliation is slow (about 8 minutes at the limit of 1,000 requests). A continuation job takes over, so no action is needed.

## Card data and PCI DSS

Card numbers, expiry dates, and security codes are entered only inside PAY.JP's iframe, which payjp.js shows. This server's page and script never touch them. The server receives only the token ID that payjp.js created and the cardholder name and email address that 3-D Secure needs.
The server stores only PAY.JP object IDs (`cus_`, `sub_`, `ch_`, Plan IDs, event IDs), amounts, periods, and states. It does not store card numbers, last four digits, or expiry dates (the card page only reads the last four digits and the expiry date from PAY.JP and shows them). Webhook bodies and PAY.JP responses are kept in neither the logs nor the database.

The payment page is served from this server's origin, so it is built with PCI DSS v4's management of payment-page scripts (requirement 6.4.3) and detection of tampering (requirement 11.6.1) in mind.

- The page loads only `https://js.pay.jp/v2/pay.js` and a small script with a nonce that is embedded in the page. It carries neither the web client's bundle nor any analytics or advertising script.
- The page always carries its own Content-Security-Policy (`script-src` only the nonce and `https://js.pay.jp`, `frame-src` only `https://js.pay.jp` and `https://api.pay.jp`, `form-action 'self'`, `frame-ancestors 'none'`), `Referrer-Policy: no-referrer`, `Cache-Control: no-store`, and `X-Content-Type-Options: nosniff`. No setting changes the CSP.
- The template and the script are embedded in the server's binary and cannot be changed at run time. Form submissions are accepted only from the same origin.

As a result, the operator's scope is expected to be that of a merchant who leaves payment entry to PAY.JP's iframe (SAQ A). Check the scope that applies and the type of self-assessment questionnaire against PAY.JP's guidance and your card brands' requirements.
When you do, it helps to keep a record of these points: the payment page URLs (`/payments/pages/payjp/checkout` and `/portal`), the list of scripts the page loads (the two above), the page's CSP, and that traffic to the server uses TLS (the reverse proxy's configuration).

## 1. Prepare the PAY.JP account

1. Create a PAY.JP account and start in test mode. Charging in live mode needs a production application and a review.
2. Check "API settings > 3-D Secure" in the PAY.JP dashboard. The server asks for 3-D Secure each time it creates a token, so you may also choose to require 3-D Secure for all token creation.
3. The page of the commerce disclosure is set in this server's administration (see [supporters.md](supporters.md)).

## 2. Keys and the webhook token

Under "API" in the PAY.JP dashboard, check these two keys. The prefix of a key sets the mode, and test keys charge nothing. Both keys must be in the same mode.

| Key | Example | Use |
| --- | --- | --- |
| Secret key | `sk_test_...` / `sk_live_...` | The server calls the API with it. It is also the source of the key that signs page URLs |
| Public key | `pk_test_...` / `pk_live_...` | payjp.js on the payment and card pages |

Pass the secret key in the environment variable `MY_PAYMENTS_PAYJP_SECRET_KEY` (or the path of a file in `MY_PAYMENTS_PAYJP_SECRET_KEY_FILE`). It cannot be written in the configuration file.
If the secret key leaks, reissue it in the dashboard and redeploy with the new key. The URLs of payment and card pages that were open before the redeploy stop working.

## 3. Register the webhook

Under "Settings > Webhook" in the PAY.JP dashboard, add a destination. Register test mode and live mode separately.

- URL: `https://<server domain>/payments/webhooks/payjp`
- Mode: the same as the keys

A PAY.JP webhook identifies its sender with the account's **origin token** (`whook_...`, in the header `X-Payjp-Webhook-Token`). Pass the token shown in the dashboard in the environment variable `MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN` (or `_FILE`).
The endpoint compares the token in constant time, and rejects a test event received with a live key (and the reverse) with `400`. There is no signature or timestamp, so anyone who knows the token can send a notification, but the server never writes state from the contents of a notification; it only reads the state back from PAY.JP.

The server uses these events (other types are accepted but not recorded).

- `subscription.created`, `subscription.updated`, `subscription.deleted`, `subscription.paused`, `subscription.resumed`, `subscription.canceled`, `subscription.renewed`
- `charge.succeeded`, `charge.failed`, `charge.refunded`
- `dispute.created`
- `statement.created`

PAY.JP resends a notification only every 3 minutes, at most 3 times, and a notification past that does not arrive until you resend it from the dashboard. The reconciliation repairs what was missed, so **setting `payments_reconcile_interval` to 1 hour is recommended**.
The state of the provider (the mode, when the last webhook arrived, and the result of the last reconciliation) is shown in the "Payment provider" section of `/admin/supporters` in the administration.

## 4. Configure the server

```sh
MY_PAYMENTS_PROVIDER=payjp
MY_PAYMENTS_PAYJP_SECRET_KEY=sk_test_...
MY_PAYMENTS_PAYJP_PUBLIC_KEY=pk_test_...
MY_PAYMENTS_PAYJP_WEBHOOK_TOKEN=whook_...
# The recommended reconciliation interval (15m to 168h, default 6h) and the most requests one reconciliation sends (default 1000)
MY_PAYMENTS_RECONCILE_INTERVAL=1h
MY_PAYMENTS_RECONCILE_MAX_REQUESTS=1000
```

`my config check` verifies the configuration. A binary built with the build tag `no_payments_payjp` refuses `payments_provider = payjp` (see [build-tags.md](../reference/build-tags.md)).

## 5. Try it in test mode

Start with test keys. Test mode and live mode have separate Plans, customers, subscriptions, webhooks, and tokens. To go live, register the webhook in live mode, replace the keys and the token with the live ones, and redeploy. The plans' Plans are created anew in the live account.

Test cards ([PAY.JP documentation](https://pay.jp/docs/testcard)):

| Number | Behavior |
| --- | --- |
| `4242 4242 4242 4242` | Succeeds |
| `4000 0000 0008 0319` | The card can be registered but the charge is declined (a failed subscription, and a failed resume after a card change) |
| `4000 0000 0000 4012` | The card can be registered but the charge fails as expired |
| `4000 0000 0000 0002` | Registering the card (creating the token) is declined |

Use a future expiry date; the security code and the name are arbitrary. In test mode, PAY.JP's test authentication screen for 3-D Secure lets you choose the result.

To try a renewal quickly, set the subscription's `trial_end` a few minutes ahead in the PAY.JP dashboard or API (PAY.JP documents this method for testing). The renewal charge happens at that time.
To try a payment failure and the grace period, subscribe, change the card on the card page to the card that is declined (`4000 0000 0008 0319`), and then let the subscription renew.

## 6. Try it locally

PAY.JP webhooks go to a public URL, so to receive them on a server on your machine, expose the port through a tunnel (cloudflared, ngrok, and the like) and register that URL as a test-mode destination.
`payjp listen --forward-to http://localhost:8080/payments/webhooks/payjp` of payjp-cli is expected to forward events as well, but whether it can forward v1 events has not been verified.

Open the payment and card pages on this server's origin (they also work on `http://localhost:8080`, but the behavior of payjp.js and 3-D Secure varies with the browser's settings).

## 7. Operation

- A reconciliation failure of kind `permission` means a wrong secret key.
- Refunds are handled in the PAY.JP dashboard. A full refund and a chargeback remove the role and delete the subscription at once, and a partial refund does not remove the role.
- **A chargeback cancellation** is not recorded automatically, because PAY.JP emits neither an event nor an API for it. When one happens, record the cancelled amount on the income screen of the costs page (`finance`). The role is not restored automatically.
- **PAY.JP's paid-plan fees and transfer fees** are not subtracted from the income. Record them as costs in `finance`.
- When reconciliations often hit the request limit, raise `payments_reconcile_max_requests` or lengthen `payments_reconcile_interval`.

## Items to verify

The following behaviors come from PAY.JP's documentation and are to be confirmed in test mode.

- How fees below one yen are handled, and how the fee returned by a refund or a chargeback is computed.
- The shape of the body of the `dispute.created` event.
- How far back the event list reaches.
- Whether deleting a Plan affects existing subscriptions (the server never deletes one), and the maximum length of a Plan ID.
- The CSP origins that the iframe type of payjp.js needs for 3-D Secure.
- Whether payjp-cli can forward v1 events.
