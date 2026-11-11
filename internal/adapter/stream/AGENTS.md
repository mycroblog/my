# Brokers of the real-time streams

- Changing the PostgreSQL bus: [PostgreSQL documentation](https://www.postgresql.org/docs/current/), search for: `NOTIFY`, `LISTEN`, `pg_notify`, 8000 bytes
- Changing the Pub/Sub bus or its subscriptions: [Pub/Sub documentation](https://docs.cloud.google.com/pubsub/docs), search for: `expirationPolicy`, `ackDeadlineSeconds`, `messageRetentionDuration`, Exactly-once delivery
- Changing the SNS topic or its subscriptions: [Amazon SNS Developer Guide](https://docs.aws.amazon.com/sns/latest/dg/llms.txt), search for: `RawMessageDelivery`, Fanout to Amazon SQS queues, `Subscribe`
- Changing the SQS queue of a listener: [Amazon SQS Developer Guide](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/llms.txt), search for: `ReceiveMessage`, long polling, `WaitTimeSeconds`
- Changing the Valkey bus: [Valkey documentation topics](https://valkey.io/topics/), search for: Pub/Sub, at-most-once, `SUBSCRIBE`, `PUBLISH`
