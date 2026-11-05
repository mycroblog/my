# mail-aws-ses

Email through Amazon SES: the identity of the sender's domain and, if you want one, a configuration set. The server uses the SES v2 API with its
execution role, so there is no password. It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It needs `app-aws-lambda`, and what that module lacks.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "mail" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/mail-aws-ses?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `from` | required | Sender address |
| `configuration_set` | null | A configuration set to create and apply to every message |

| Output | Value |
| --- | --- |
| `env` | `MY_MAIL`, `MY_MAIL_FROM`, and `MY_MAIL_SES_CONFIGURATION_SET` with a set |
| `secret_env` | Empty |
| `permissions` | `ses:SendEmail` on the identity, and on the set |
| `dns_records` | The DKIM records to add. This output is not part of the backend contract: read it from this module, not through `backends` |

A new SES account can send only to verified addresses until AWS grants production access, on request, which no configuration can do for you.
The bounces and complaints need an SNS topic and `mail_ses_sns_topic_arn`, which this module does not create.
