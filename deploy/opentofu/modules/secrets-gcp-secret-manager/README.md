# secrets-gcp-secret-manager

Secrets in Secret Manager. You add the values that come from elsewhere by hand, the first apply can generate a VAPID key, and another
module's value can be written in through an ephemeral input, so this module keeps no secret value in the plan or the state. It needs
OpenTofu 1.11 or later, and follows [the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "secrets" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/secrets-gcp-secret-manager?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id` | required | Project of the secrets |
| `secrets` | `{}` | Settings this module provides: variable name to secret ID, such as `MY_VAPID_PRIVATE_KEY = "my-vapid-key"` |
| `referenced` | `{}` | Secrets another module sets its own setting from: any key to secret ID |
| `generate_p256_keys` | `[]` | Keys of `secrets` whose value the first apply generates: a P-256 private key as 32 bytes in base64url, the format of `vapid_private_key`. Decide before the first apply: added later, it writes a new latest version over the value you have |
| `write_keys` | `[]` | Keys of `secrets` or `referenced` whose value comes from `write_values`, such as the URLs of `db-neon` |
| `write_values` | `{}` | The value of each key of `write_keys`. An ephemeral input: this module keeps none of it, whatever the module it comes from keeps |
| `write_version` | `1` | Raise it to write `write_values` again, after a value changed where it comes from. Nothing else can tell |

| Output | Value |
| --- | --- |
| `env` | Empty |
| `secret_env` | `secrets`: variable name to secret ID |
| `permissions` | Empty: the app module grants access to every secret it is given |
| `ids` | Every secret ID by its key, for `db-external`, `mail-smtp`, and the like |

Use `referenced` when another backend names the setting, so that no setting is named twice:

```hcl
module "secrets" {
  source     = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/secrets-gcp-secret-manager?ref=v0.1.0&depth=1"
  project_id = var.project_id
  secrets    = { MY_VAPID_PRIVATE_KEY = "my-vapid-key" }
  referenced = { smtp_password = "my-smtp-password" }
}

module "mail" {
  source             = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/mail-smtp?ref=v0.1.0&depth=1"
  password_secret_id = module.secrets.ids["smtp_password"]
  # ...
}
```

Add each value before anything uses the secret, since an instance whose secret cannot be read does not start:

```sh
printf '%s' '<the value>' | gcloud secrets versions add my-smtp-password --project="$PROJECT" --data-file=-
```

A generated value goes through an ephemeral resource and a write-only attribute. Its version is fixed in the module and has
`prevent_destroy`, because a new key ends every Web Push subscription. Any 32 bytes make a P-256 key except about 1 in 2^32,
which `my config check` would report.

A secret made elsewhere needs nothing here: pass its ID to the module that uses it, or to `secret_env` of the app module.
