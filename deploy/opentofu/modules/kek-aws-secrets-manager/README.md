# kek-aws-secrets-manager

The KEK in AWS Secrets Manager, generated on the first apply. It follows [the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kek" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kek-aws-secrets-manager?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `secret_name` | `my-kek` | Name of what it creates |
| `secret_arn` | null | ARN of a secret made elsewhere. Set, nothing is created |

| Output | Value |
| --- | --- |
| `env`, `permissions` | Empty |
| `secret_env` | `MY_KEK`: the ARN of the secret |

On Google Cloud the KEK is a file, because a mounted secret is the safer form that Cloud Run offers. Here it is `secret_env`: the swap of this block
for `kek-aws-kms` is what holds, and a block of the other platform fails the plan.

## What protects the value

The value goes through `ephemeral "random_bytes"` and `secret_string_wo`, with `secret_string_wo_version` fixed at 1, **never to be changed**:
a new number writes a new random KEK, and everything the old one encrypted becomes unreadable. Rotate with `my kek rewrap`. The provider declares
`secret_string_wo` as write-only, and `scripts/tofu-aws-plan-test.sh` shows that the plan holds nothing of it. **No apply has been run**, not even against
a stub, so that the state holds nothing is what the provider's schema promises rather than what has been seen.

The secret and its version have `prevent_destroy`. A version has nothing like Google's `deletion_policy = "ABANDON"`: if it leaves the configuration by
any way that skips `prevent_destroy`, the provider deletes it. A deleted secret is kept for a recovery window, 30 days by default. Copy the KEK somewhere
safe after the first apply, as the guide for Cloud Run says.
