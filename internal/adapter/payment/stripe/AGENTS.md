# Stripe API (stripe-go v87, API version 2026-10-28.endive)

- Verifying a webhook signature or changing its tolerance: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: `Stripe-Signature`, Verify webhook signatures manually, Preventing replay attacks
- Adding or removing an event type the adapter reads: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: Using webhooks with subscriptions, `customer.subscription.updated`, `invoice.paid`, `invoice.payment_failed`
- Changing how a subscription is cancelled, paused, resumed, or changed at the period's end: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: Cancel subscriptions, `cancel_at_period_end`, Subscription schedules, `phases`, `proration_behavior`
- Changing the hosted Checkout Session (subscription or one-time payment): [Stripe documentation](https://docs.stripe.com/llms.txt), search for: Create a Checkout Session, `subscription_data`, `payment_intent_data`, `adaptive_pricing`, Expire a Checkout Session
- Changing the customer portal link: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: customer portal, Create a portal session, `return_url`
- Converting amounts or adding a currency: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: Zero-decimal currencies, Special cases, Minimum and maximum charge amounts
- Reading refunds, disputes, and fees as money movements: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: List all balance transactions, `charge.dispute.funds_withdrawn`, `refund.updated`, `reporting_category`
- Changing the reconciliation that lists events and subscriptions: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: List all events, 30 days, `starting_after`, `has_more`
- Changing the request rate or the retries: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: Rate limits, `Stripe-Rate-Limited-Reason`, Idempotent requests, `Idempotency-Key`
- Upgrading stripe-go, which moves the pinned API version: [Stripe documentation](https://docs.stripe.com/llms.txt), search for: API upgrades, Set a Stripe API version, `Stripe-Version`, Changelog
- Relying on the order or the count of webhook deliveries (events may arrive out of order and more than once): [Stripe documentation](https://docs.stripe.com/llms.txt), search for: Event ordering, Handle duplicate events, Retry behavior
