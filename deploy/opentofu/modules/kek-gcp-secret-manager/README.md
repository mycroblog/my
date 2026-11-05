# kek-gcp-secret-manager

The KEK in Secret Manager (`key_manager = local`), which reaches the server, the migration job, and the console as a mounted
file named by `MY_KEK_FILE`, never as an environment variable. It follows
[the backend contract](../../README.md#the-backend-contract).

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "kek" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/kek-gcp-secret-manager?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id` | required | Project of the secret |
| `secret_id` | `my-kek` | ID of the secret |
| `create` | `true` | Creates the secret and generates the KEK. Set it to `false` for a secret made elsewhere, whose value you manage |

| Output | Value |
| --- | --- |
| `secret_files` | `MY_KEK_FILE`: the secret to mount |
| `env`, `secret_env`, `roles` | Empty: the app module grants access to the secret |

The first apply generates 32 random bytes with an ephemeral resource and sends them through a write-only attribute, so the
value reaches neither the plan nor the state, and no later apply writes another. It needs OpenTofu 1.11 or later.

**Copy the KEK right after the first apply**, and keep the copy apart from the database, as
[the guide](../../../../docs/operations/deploy-cloud-run.md#4-create-the-secrets-and-add-their-values) shows. Secret
Manager is the only other place it exists, and a backup is useless without it.

**This resource does not rotate the KEK.** `secret_data_wo_version` is fixed in the module, on purpose: changing it writes a
new random KEK as the latest version, and everything the old one encrypted becomes unreadable, every account's private key
among it. The secret and its version have `prevent_destroy` for the same reason, and `destroy` leaves them alone until you
remove that yourself. To rotate, follow [Rotate the KEK](../../../../docs/operations/key-management.md#rotate-the-kek).

Replacing this module on an instance that already has data is a move, not a swap: see
[the guide](../../../../docs/operations/deploy-cloud-run.md#changing-the-kek-backend).
