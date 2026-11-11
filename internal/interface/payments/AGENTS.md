# Webhook endpoints of the payment providers

- Changing the status code or the time the endpoint takes to answer Stripe: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: `2xx`, Retry behavior, Handle duplicate events
- Changing the answer to a PAY.JP delivery: [PAY.JP v1 guides](https://docs.pay.jp/v1/), search for: Webhook, `X-Payjp-Webhook-Token`
