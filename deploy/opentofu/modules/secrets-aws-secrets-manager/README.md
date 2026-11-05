# secrets-aws-secrets-manager

Secrets in AWS Secrets Manager: ones the operator fills by hand, and a VAPID key that it can generate. It follows
[the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It lacks a server that reads them: `app-aws-lambda` says why.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "secrets" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/secrets-aws-secrets-manager?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `secrets` | empty | Secret settings by variable name, to the name of the secret |
| `referenced` | empty | Secrets another module takes, such as `db-external` |
| `generate_p256_keys` | empty | Keys of `secrets` whose value the first apply generates |
| `write_keys` | empty | Keys of `secrets` or `referenced` whose value comes from `write_values`, such as the URLs of `db-neon` |
| `write_values` | empty | The value of each key of `write_keys`. An ephemeral input: this module keeps none of it, whatever the module it comes from keeps |
| `write_version` | `1` | Raise it to write `write_values` again, after a value changed where it comes from. Nothing else can tell |

| Output | Value |
| --- | --- |
| `env`, `permissions` | Empty |
| `secret_env` | Variable name to the secret's ARN |
| `ids` | The ARN of every secret by its key |

A secret is named by its ARN, because Secrets Manager adds a random suffix to it and IAM grants by the ARN. An ARN is known only once the secret
exists, which is fine here: the app module of AWS keys nothing on it.

Add a value with `aws secretsmanager put-secret-value --secret-id <ARN> --secret-string file://<a file>`, so that it is not in your shell's history.

## What is generated

The value goes through `ephemeral "random_bytes"` and `secret_string_wo`, with `secret_string_wo_version` fixed at 1. The provider declares
`secret_string_wo` as write-only. `scripts/tofu-aws-plan-test.sh` shows that the plan holds nothing of it. **No apply has been run**, not
even against a stub, so that the state holds nothing is what the provider's schema promises rather than what has been seen.

A destroy is refused for a generated version. Secrets Manager keeps a deleted secret for a recovery window, 30 days by default, in which its
name cannot be used again.
