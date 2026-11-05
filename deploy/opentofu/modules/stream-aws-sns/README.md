# stream-aws-sns

The broker of the real-time updates: an SNS topic, to which the server subscribes one SQS queue per listening process, which it
creates and deletes itself. It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It needs `app-aws-lambda`, and what that module lacks.

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
| `roles` | What the server's `sns` adapter documents, the `sns:Subscribe` of it with its conditions |

## On Lambda

A stream holds a whole instance of the web function for up to 15 minutes, and a listener makes SQS requests all that time: three a minute, and
about seven streams open at all times use up SQS's free million a month. Whether a deployment on Lambda should offer streams by default is
an open question of the project, so leave this block out of `backends` and set `MY_STREAM` yourself if it does not.

Whether SNS delivers to a queue of a process that Lambda has frozen between invocations, within the queue's 60 seconds, is unverified.
