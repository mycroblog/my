# Managing the key-encryption key (KEK)

A guide for operators. The [configuration reference](../reference/configuration.md) lists every setting.

## How it works

- Secrets stored in the database, such as actors' private keys, are encrypted with AES-256-GCM using a data-encryption key (DEK) generated for each secret.
  The DEK is wrapped with a **key-encryption key (KEK)** and stored next to the ciphertext (envelope encryption).
- The KEK is not stored in the database, so a leaked database dump alone does not expose the secrets.
- Each ciphertext carries the **key ID** of the KEK that wrapped its DEK. With `key_manager = local`, the key ID is `local:` followed by 16 hexadecimal digits derived from the KEK,
  so the same KEK always gives the same ID. You never assign IDs.
- A ciphertext is bound to the row ID and column name where it is stored. Copying it to another row or column makes it undecryptable.
- The signing key (HMAC-SHA256) for media proxy URLs is a random key that the server generates on its first start and stores in the database, encrypted like the other secrets.
  It does not change when you rotate the KEK, so media proxy URLs keep working across rotations, and `my kek rewrap` re-wraps it with the other secrets.
- `key_manager` is `local` (it uses the `kek` setting) only. The settings `kms_key`, `kms_key_previous`, `kms_dek_cache_ttl`, and `kms_dek_cache_entries`
  are for the KMS key managers (`gcpkms`, `awskms`), which this version does not offer yet; with `key_manager = local` they have no effect.

## What happens if you lose the KEK

Without the KEK, restoring the database does not let you decrypt actors' private keys. Once a private key is lost, other servers can no longer verify that actor's signatures,
and **the actor's identity in the fediverse is broken** (followers' relationships cannot be carried over). Treat the KEK with the same care as the database.

## Generate a KEK

Encode 32 random bytes with standard base64.

```sh
openssl rand -base64 32
```

Where `openssl` is not available, `head -c 32 /dev/urandom | base64` works too. Do not leave the generated value in your terminal history or in tickets.

## Store and deliver it

The KEK is a secret setting, so it can be passed only through the environment variable `MY_KEK`, or through `MY_KEK_FILE`, the path of a file holding the value. Putting it in the configuration file (TOML) prevents startup.

| Target | Example of how to deliver it |
| --- | --- |
| Self-hosted (binary / systemd) | Put it in a file only the owner can read (`chmod 600`) and point `MY_KEK_FILE` at it |
| Docker / Compose | Mount a Docker secret at `/run/secrets/my_kek` and set `MY_KEK_FILE=/run/secrets/my_kek` |
| Kubernetes / microk8s | Mount a Secret as a file and point `MY_KEK_FILE` at it |
| Cloud Run | Expose a Secret Manager secret as a file or an environment variable |
| AWS Lambda / ECS | Pass it from Secrets Manager or SSM Parameter Store (SecureString) |

- Give every process, such as web and worker, the same KEK.
- The KEK never appears in logs or error messages (only the key ID does).
- `my config check` validates the KEK's format (base64 of 32 bytes) and prints the value as `(redacted)`. In the `production` profile it also fails on a KEK
  that is not random, such as all zeros or a repeated byte, and `my serve` logs a warning about one (see the [production checklist](production-checklist.md)).

## Back it up

**Back up the database and the KEK, separately.**

- Keep the KEK somewhere other than where the database backups live (a password manager, offline media, a secret store in another account, and so on).
  If both are in one place, a leak of that place makes the database encryption pointless.
- During a rotation, include the `kek_previous` keys in the backup for as long as ciphertext wrapped with them remains in the database.
- Include the KEK in your restore procedure, and test restores regularly ([Backup and restore](backup.md#recovery-drill)). After restoring a database, set the KEK (and `kek_previous`) that was in use at backup time to the same values.
  With a different KEK, anything that reads a secret fails with an `unknown key-encryption key` error that names the key ID.

## Rotate the KEK

A KEK in `kek_previous` is used only for decryption, and new secrets are always encrypted with `kek`.
So that a rolling deploy, where old and new processes run side by side, never produces ciphertext that some process cannot read, switch in two steps.

1. Generate a new KEK and back it up.
2. Restart every process with `MY_KEK=<old KEK>` and `MY_KEK_PREVIOUS=<new KEK>`. Now every process can decrypt with the new KEK.
3. Restart every process with `MY_KEK=<new KEK>` and `MY_KEK_PREVIOUS=<old KEK>`. From now on secrets are wrapped with the new KEK, and ciphertext of the old KEK is still readable.
4. Re-wrap the stored secrets with the new KEK: run `my kek rewrap` once, with the same configuration as step 3 (see [Re-wrap the stored secrets](#re-wrap-the-stored-secrets)).
   When it exits with 0, no secret in the database needs the old KEK any more.
5. Wait at least 30 minutes after step 3, so that the short-lived sign-in tokens issued with the old KEK have expired, then restart every process with `MY_KEK=<new KEK>` and no `kek_previous`.
   Keep the backup of the old KEK for as long as you keep database backups taken before step 4: restoring one of them needs it again.

`kek_previous` accepts several comma-separated keys. Listing the same key twice, or listing the same key as `kek`, prevents startup.

### Re-wrap the stored secrets

`my kek rewrap` replaces only the wrapped data-encryption key of each secret and keeps its ciphertext; it never writes or prints a secret.

```sh
my kek status   # count the stored secrets by the key ID of the KEK that wraps them; decrypts nothing
my kek rewrap   # re-wrap every secret that a key of kek_previous wraps with kek
```

- `my kek status` prints one line per column and key ID, marks the key ID of `kek` with `(current)`, and ends by telling whether any secret still needs a key of `kek_previous`.
  A key ID that is neither `kek` nor in `kek_previous` means a KEK is missing from the configuration.
- `my kek rewrap` reads the secrets in batches of 100 and writes each batch in one short transaction. It prints, per column, how many secrets it re-wrapped and how many were already wrapped with `kek`.
  It can run while the server is serving, and it is safe to stop or to run again at any time: it skips the secrets already wrapped with `kek`, and a secret that the server rewrote in the meantime is left as the server wrote it.
- A secret that cannot be re-wrapped is left unchanged and reported with its row ID, and the command exits with 1. Either its KEK is not configured
  (the message names the key ID: add that KEK to `kek_previous` and run it again), or the stored value is damaged or was copied from another row.
  Do not remove a KEK from `kek_previous` while `my kek status` still counts secrets under it.

## Move between the local KEK and a KMS

A key ID starts with the kind of key manager that wrapped it (`local:`, `gcpkms:`, `awskms:`). The server wraps new secrets with the current `key_manager`
and unwraps each stored secret with the key manager its key ID names, so a move between the local KEK and a KMS is a rotation across kinds.
This version offers only `key_manager = local`; the steps below apply once a KMS key manager is available.

- With a KMS, `kek` and `kek_previous` only decrypt: they open the secrets still wrapped with the local KEK, and every new secret is wrapped with `kms_key`.
  `kms_key_previous` lists earlier keys of the same KMS, which only decrypt too.
- From the local KEK to a KMS: set `key_manager` and `kms_key`, keep the current KEK in `kek` (or move it to `kek_previous`), restart every process, and run `my kek rewrap`.
  When `my kek status` no longer counts secrets under a `local:` key ID, remove `kek` and `kek_previous`.
- From a KMS back to the local KEK, do the same the other way round: the KMS stays configured, only to decrypt, until `my kek rewrap` has moved every secret.
- To move from one cloud's KMS to another's, move to the local KEK first and then to the other KMS; there is no direct move.
- The server keeps a data-encryption key that the KMS decrypted for `kms_dek_cache_ttl` (default `10m`, at most `kms_dek_cache_entries`, default `1024`),
  so a secret in use reaches the KMS about once per period and process. `0` in either setting disables the cache.

## If the KEK leaks

If only the KEK leaked, rotate it as above. If both the KEK and the database may have leaked, rotation does not protect you:
the actors' private keys in the database are already readable, so the actor keys themselves must be regenerated.

## Web Push VAPID key

Web Push is signed with the server's VAPID key (P-256, RFC 8292). It is a secret separate from the KEK and is not stored in the database.

- Create a key pair with `my config vapid-key`, and give every process the private key (the value of `vapid_private_key = ...`) through `MY_VAPID_PRIVATE_KEY` or `MY_VAPID_PRIVATE_KEY_FILE`.
  The server derives the public key from the private key and hands it to clients, so you do not configure it.
- If you do not set it, Web Push is disabled and clients do not offer it. In the `production` profile, `my config check` and the start of `my serve` warn about that.
- Browsers bind a subscription to the public key, so changing the key stops delivery to existing subscriptions. When a user turns push notifications on again in settings, the client subscribes again with the new key.
  Do not change the key unless it leaked; if it leaks, switch to a new key and ask users to turn notifications on again.
