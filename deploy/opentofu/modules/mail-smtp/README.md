# mail-smtp

Email through any SMTP service, so its name has no provider part. It creates nothing, and follows
[the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "mail" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/mail-smtp?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `from` | required | Sender address, as a bare address. Give its domain SPF, DKIM, and DMARC records |
| `host` | required | SMTP server, which must offer STARTTLS with a certificate valid for this name |
| `port` | `587` | Submission port. Google Cloud blocks outbound port 25 |
| `username`, `password_secret_id` | null | SMTP AUTH: set both, or neither for a server that needs no authentication. The password is the ID of a secret you created by hand |

| Output | Value |
| --- | --- |
| `env` | `MY_MAIL`, `MY_MAIL_FROM`, `MY_MAIL_SMTP_HOST`, `MY_MAIL_SMTP_PORT`, and `MY_MAIL_SMTP_USERNAME` when set |
| `secret_env` | `MY_MAIL_SMTP_PASSWORD` when set |
| `permissions` | Empty: the app module grants access to every secret it is given |

To send no email, leave this module out of `backends`: `mail` then stays at its default, `none`.
