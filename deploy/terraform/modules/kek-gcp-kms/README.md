# kek-gcp-kms

The KEK in Cloud KMS, which it never leaves (`key_manager = gcpkms`). A leaked database dump and a leaked configuration
together still do not expose the secrets. It follows [the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kek" {
  source = "git::https://github.com/mycroblog/my.git//deploy/terraform/modules/kek-gcp-kms?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id`, `location` | required | Where the key goes |
| `key_ring`, `key` | `my`, `my-kek` | Names of what it creates |
| `crypto_key_id` | null | Resource name of a key made elsewhere, without a version. Set, nothing is created |

| Output | Value |
| --- | --- |
| `env` | `MY_KEY_MANAGER`, `MY_KMS_KEY` |
| `secret_env` | Empty |
| `roles` | `roles/cloudkms.cryptoKeyEncrypterDecrypter` on the key, which the service and the jobs share through one service account |

Enable `cloudkms.googleapis.com` first. The key has `prevent_destroy`, because destroying it loses every secret it wrapped.
Cloud KMS keeps key rings and keys in any case: a key can only have its versions destroyed.

Replacing a KEK module on an instance that already has data is a move, not a swap: see
[the guide](../../../../docs/operations/deploy-cloud-run.md#changing-the-kek-backend).
