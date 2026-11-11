# PAY.JP API v1 (subscriptions) and v2 (one-time payments)

- Changing a plan, a subscription, or its pause, resume, and cancel commands (v1 only: v2 has no subscriptions): [PAY.JP API v1 reference](https://docs.pay.jp/v1/api/), search for: `/v1/subscriptions`, `pause`, `resume`, `cancel`, `trial_end`, `prorate`
- Reading charges, refunds, and statements as money movements: [PAY.JP API v1 reference](https://docs.pay.jp/v1/api/), search for: `/v1/charges`, `amount_refunded`, `/v1/statements`, `captured`
- Changing the reconciliation over the Events API: [PAY.JP API v1 reference](https://docs.pay.jp/v1/api/), search for: `/v1/events`, `subscription.renewed`, `charge.failed`, `pending_webhooks`
- Verifying a webhook or changing the answer to one: [PAY.JP v1 guides](https://docs.pay.jp/v1/), search for: Webhook, `X-Payjp-Webhook-Token`
- Changing the card form of the checkout and portal pages (payments.js, 3-D Secure): [PAY.JP v1 guides](https://docs.pay.jp/v1/), search for: `payjp.js`, `threeDSecureWorkflow`, `createToken`
- Changing the request rate or the back-off: [PAY.JP API v1 reference](https://docs.pay.jp/v1/api/), search for: Rate Limit, `over_capacity`, 429
- Changing the one-time payment (Checkout Session, PaymentFlow, refunds, disputes): [PAY.JP v2 documentation](https://docs.pay.jp/v2/llms-full.txt), search for: Checkout Session, Payment Flow, `Idempotency-Key`, Webhook
