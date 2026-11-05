# stream-gcp-pubsub

The broker of the real-time updates: a Pub/Sub topic, and a custom role that lets the server manage its own subscriptions.
It follows [the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "stream" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/stream-gcp-pubsub?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id` | required | Project of the topic and the subscriptions |
| `name` | `my-stream` | The topic and the start of the subscription IDs, used by this instance alone |

| Output | Value |
| --- | --- |
| `env` | `MY_STREAM`, `MY_STREAM_PUBSUB_TOPIC`, `MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX` |
| `secret_env` | Empty |
| `permissions` | `roles/pubsub.publisher` and `roles/pubsub.subscriber` on the topic, and the custom role on the project |

The server creates one subscription per process that has open streams, and deletes it. The periodic job `stream.sweep`
removes what ended processes leave behind. Those names are not known in advance, so the custom role
(`pubsub.subscriptions.create`, `list`, `consume`, `delete`) goes on the project. Keep the project to this instance, or
the role reaches other subscriptions too.

Google Cloud keeps a deleted custom role for a while and refuses its ID, so a role deleted by `destroy` needs another
`name`, or undeleting.
