# stream-aws-sns

The broker of the real-time updates: an SNS topic, to which the server subscribes one SQS queue per listening process, which it
creates and deletes itself. It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "stream" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/stream-aws-sns?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `name` | `my-stream` | The topic and the start of the queues' names, used by this instance alone. At most 40 characters |

| Output | Value |
| --- | --- |
| `env` | `MY_STREAM`, `MY_STREAM_SNS_TOPIC_ARN`, `MY_STREAM_SNS_QUEUE_PREFIX` |
| `secret_env` | Empty |
| `permissions` | What the server's `sns` adapter documents, the `sns:Subscribe` of it with its conditions |
