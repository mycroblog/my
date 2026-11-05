# kek-aws-kms

The KEK in AWS KMS, which it never leaves (`key_manager = awskms`). It follows [the backend contract](../../README.md#the-backend-contract).
Of the AWS modules this one asks least of the server: the `awskms` adapter exists and reads no secret, so it needs no KEK from Secrets Manager.

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. It needs `app-aws-lambda`, and what that module lacks, though `database_url` is still a secret.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kek" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/kek-aws-kms?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `alias` | `my-kek` | Alias of the key it creates |
| `key_arn` | null | ARN of a key made elsewhere. Set, nothing is created |

| Output | Value |
| --- | --- |
| `env` | `MY_KEY_MANAGER`, `MY_KMS_KEY` |
| `secret_env` | Empty |
| `roles` | `kms:Encrypt`, `kms:Decrypt`, and `kms:DescribeKey` on the key |

The key has `prevent_destroy` and a deletion window of 30 days, and rotates its key material. It costs a monthly fee, and a fee per request beyond a free
tier: [Key management](../../../../docs/operations/key-management.md#aws-kms) has more.
