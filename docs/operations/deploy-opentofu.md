# Deploying with OpenTofu

What every deployment with the OpenTofu root modules shares: [Deploying on Google Cloud](deploy-google-cloud.md) and
[Deploying on AWS](deploy-aws.md) add what their platform needs. Both modules need
[OpenTofu](https://opentofu.org) 1.11 or later. Terraform is not supported.

## The root modules

One block of a root module deploys the whole server: the service, its jobs, and a backend for each role.

| Root module | Platform | Example to copy |
| --- | --- | --- |
| `mycroblog-gcp` | Cloud Run on Google Cloud | `examples/gcp-cloud-run-neon` |
| `mycroblog-aws` | ECS on Fargate, on AWS. It has never been applied | `examples/aws-ecs-neon` |

An example is the platform's provider and one block, `module "mycroblog"`, whose source is a Git source pinned to a
release:

```hcl
module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host = "social.example.org"
  # the sections, below
}
```

The pin is required: an unpinned module could change the KEK's resources under you. The server image defaults to the
release that `ref` names, so `ref` alone names what runs, and raising it is an upgrade. The module exists only from the first
release that contains it.

### Sections

Each role is a section of the block, an object whose `kind` picks the backend, such as `database = { kind = "external" }`.
Every field has a default, the recommended setup, so you write only what you change. A combination that cannot work fails
before the plan, with a message that names the inputs. [The modules' reference](../../deploy/opentofu/README.md#sections)
lists each module's sections, kinds, and fields.

### Other settings and secrets

`settings` takes any other setting of the [configuration reference](../reference/configuration.md) by its key, and the
module passes it to the service, the workers, and the jobs. `secrets` takes the keys of secret settings: for each, the
module creates an empty secret `<name>-<key>`, with hyphens for underscores, passes it to the server, and lists it to fill.

```hcl
  settings = { translator_deepl_url = "https://api.deepl.com" }
  secrets  = ["translator_deepl_api_key"]
```

A key that a section sets or the module fixes fails the plan, and so does a key the module does not take, such as `listen`:
the error gives the reason. A section that is null, such as `translation`, and a signal of `observability` set to `settings`
set nothing, so `settings` may name their keys. Error reports to Sentry, for example:

```hcl
  observability = { errors = "settings" }
  settings      = { errors_exporter = "sentry" }
  secrets       = ["errors_sentry_dsn"]
```

Settings changed by hand on the platform do not last: the next apply puts back what the block says.

### Secrets

No secret value you give reaches the OpenTofu plan or state. The module generates what it can through write-only
attributes: the KEK, the VAPID key, and the password of a database it creates. The one exception is the origin secret of
`ingress.kind = cloudflare`, which Cloudflare's rule holds, so it lands in the state, which must stay private.

The secrets only you have, such as the URL of an external database, an SMTP password, an access key, or the keys of
`secrets`, the module creates empty. `tofu output secrets_to_fill` lists each, with what goes in it and the command that
adds it. A server whose secret has no value does not start. The server reads its secrets when an instance or a task starts,
so a new value takes effect on the next one.

**OpenTofu generates the KEK once and never again.** The secret and its version have `prevent_destroy`, and the version that
would write a new value is fixed in the module: a new random KEK makes everything the old one encrypted unreadable. To
rotate it, follow [Rotate the KEK](key-management.md#rotate-the-kek), adding the new value with the platform's CLI, which
OpenTofu does not track.

## First deployment

You need `tofu` 1.11 or later, `jq`, and the platform's CLI (`gcloud` or `aws`) where you apply, since an apply runs the migration from
there; a domain for [`host`](../reference/configuration.md#host), which cannot change later
([Settings you cannot change later](production-checklist.md#settings-you-cannot-change-later)); and a version of `my` from
the [releases](running.md#get-the-image). There is no `latest` tag. Do first what the platform's page asks, such as creating
the database.

### 1. Copy the example and set the variables

Copy the example, with its provider lock file, from the release you deploy into an empty directory:

```sh
VERSION=v0.1.0
EXAMPLE=gcp-cloud-run-neon # or aws-ecs-neon
git clone --depth 1 --branch "$VERSION" https://github.com/mycroblog/my.git my-source
mkdir my-deployment
cp -r "my-source/deploy/opentofu/examples/$EXAMPLE/." my-deployment/
rm -rf my-source
cd my-deployment
cp terraform.tfvars.example terraform.tfvars
```

**Set `ref` to that release** in `main.tf`, which a stable release's copy already has right. For a prerelease, set both the
`ref` and the image, the version without its `v`, since the module of a prerelease does not default to its own image:
`sed -i "s/?ref=[^&]*/?ref=$VERSION/" main.tf`, and `image = { version = "0.1.0-rc.4" }` in the block.

Fill in `terraform.tfvars`, and add to the block in `main.tf` any section you change. Decide on
[`account_domain`](../reference/configuration.md#account_domain) now, as `settings = { account_domain = "example.org" }`, if
handles use another domain.

Keep this directory: it holds your settings and, by default, the state. The state holds no secret values but is the only
record of what was created: keep it in a remote backend, such as a `gcs` or `s3` bucket, if more than one person applies.

### 2. Create the secrets and add their values

```sh
tofu init
SCRIPTS=.terraform/modules/mycroblog/deploy/opentofu
```

`tofu init` fetches the module and its scripts at that release, under the name of your block: `migrate.sh`, which an apply
runs, and `console.sh`, which runs [the console](#the-console).

Apply the secrets alone first, which creates them and generates the KEK and the VAPID key, with the module of the KEK's kind:

| Root module | Apply |
| --- | --- |
| `mycroblog-gcp` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secret_manager` |
| `mycroblog-aws` | `tofu apply -target=module.mycroblog.module.secrets -target=module.mycroblog.module.kek_secrets_manager` |

With `kek = { kind = "kms" }`, target `module.mycroblog.module.kek_kms` in place of the KEK's secret.

**Copy the KEK now**, and keep the copy apart from the database, for example in a password manager: without it a
[backup](backup.md) cannot be restored, and the accounts' keys are lost. `tofu output kek` has the command. Copy the VAPID
key, `<name>-vapid-key`, the same way: its loss ends every Web Push subscription.

Then run each command of `tofu output secrets_to_fill` with its value. Add them all before the next step.

### 3. Apply

```sh
tofu apply
```

This creates the migration's job with what it depends on, runs `my upgrade` in it through `migrate.sh`, and waits for it:
`my config check`, `my db upgrade -check`, `my schema apply`, and `my db upgrade`, which stops at the first that fails. Its
output appears in the apply's. The first command that uses the database records `host` and `account_domain` in it. Only then
does the apply create the services, whose health check fails until the schema exists.

If the migration fails, the apply stops before the services. Fix the cause, often a secret without a value, and run
`tofu apply` again: it runs the migration again.

### 4. Point the domain at the server

`tofu output dns_records` lists the records to add at your DNS provider, and is empty where the module creates them.

### 5. Create the first administrator

The console has no terminal and no standard input, so `my admin create` prints a link that sets the password instead of
reading one:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" admin create -username alice -email alice@example.com -password-link
tofu apply -var console_enabled=false
```

Open the link right away and set the password. It works once, for one hour, and **stays in the platform's logs until then**,
where anyone who can read them can use it. If it expires, run `my admin account password-reset-link @alice` through the
console. Then turn on two-factor authentication.

### 6. Go through the checklist

Go through the [production checklist](production-checklist.md), starting with [email](production-checklist.md#email): SPF,
DKIM, and DMARC on the domain of `mail.from`.

## The console

The console runs one `my` command with the image, settings, and secrets of the service. It exists only while
`console_enabled` is on, so turn it off when you finish:

```sh
tofu apply -var console_enabled=true
tofu output -json console | "$SCRIPTS/console.sh" instance set registration_mode open
tofu apply -var console_enabled=false
```

`console.sh` waits for the command, prints its output, which it reads back from the platform's logs, and exits with its exit
code. It uses the image the service runs: after an upgrade that stopped at the migration, that is the previous release, which
the database still fits.

The command has no standard input and no terminal, so a command that reads a password or a file from them cannot run this
way. What it prints stays in the logs, so do not run a command whose output you would not write there.

## Upgrading

Read [Upgrading](database.md#upgrading) and the release notes first, and go one minor version at a time. An upgrade is an
apply of the new release:

```sh
# 1. Back up the database while nothing writes: see "The maintenance mode" below, and Backup and restore.
pg_dump --format=custom --file=my-before-upgrade.dump "$DATABASE_URL"

# 2. Name the new release in main.tf, as ?ref=v0.2.0, and fetch it.
tofu init

# 3. Read the plan, then apply it.
tofu plan
tofu apply
```

Read the plan: a new release's module may change resources too. The apply does this, and stops at the first step that fails:

1. Gives the migration and console jobs (task definitions on AWS) the new image, with the rest of the plan that the services do not depend on.
2. Runs `my upgrade` in the migration job through `migrate.sh`, from where you apply, and waits for it, as in the
   [first deployment](#3-apply). It first waits for any run that an interrupted apply left going.
3. Moves each worker to the new image, then the service. A new revision takes traffic only once its health check passes.

**Where you apply** needs `jq`, the platform's CLI, and credentials that may run the job and read its logs, which the
platform's page lists. The same holds for a CI job that applies.

**When a step fails**, the services stay on their revisions, and the release notes or the migration's output say why. Fix
the cause and run `tofu apply` again with the same version: it runs the migration again, and the steps that completed find
nothing left to do. An apply of the same image runs nothing. To go back after the schema changed, restore the backup: see
[Rolling back](database.md#rolling-back). An apply of an earlier version stops at the migration, since that release refuses
a newer schema, and changes no service.

To try a release candidate, or to keep the image of an earlier release under a newer module, set the version yourself, the
image tag without the `v` of the release:

```hcl
  image = { version = "0.2.0-rc.1" }
```

### Checking the SQL first

The plan does not show the SQL of the migration. To read it before the apply, apply the migration job alone with the new
image, run `my upgrade -dry-run` in it, which changes nothing, and read its logs. The platform's page has the commands.

### Destructive changes

When the release notes call for `-allow-destructive`, name that version in the block:

```hcl
  upgrade = { allow_destructive_for = "0.3.0" }
```

`my upgrade` gets `-allow-destructive` only on the apply of that version, so a value left in place lets no later release
drop data.

### Pinning the image by digest

The [production checklist](production-checklist.md#operations) recommends a digest, from the release notes:

```hcl
  image = { digest = "sha256:..." }
```

A digest names one image, whatever the tag beside it says, so change it in the same edit as `ref`: a digest left from the
previous release keeps that release's image under the new version.

### The maintenance mode

The [maintenance mode](maintenance.md) takes three applies, since the services change before the upgrade and after it. Add
it to the settings of the block and apply, take the backup, raise `ref` and apply, then remove it and apply:

```hcl
  settings = { maintenance = "true" }
```

Without it, the old revision keeps serving during the migration, which
[is not guaranteed to work](database.md#the-server-during-an-upgrade).

## Splitting jobs into workers

When one kind of job holds up the rest, give it its own service. Decide what to split from the metrics, as
[Splitting workers by job type](running.md#splitting-workers-by-job-type) says. `workers` takes a map by a key that names each
worker, with its `job_types`, and the module makes every queue, route, and setting of the split from it, so you write none
of them. To keep deliveries from delaying the rest:

```hcl
  workers = {
    delivery = { job_types = ["federation.fanout", "federation.deliver"] }
  }
```

To keep email and notifications quick:

```hcl
  workers = {
    notify = {
      job_types = [
        "identity.verification_email", "identity.password_reset_email", "identity.password_notice_email",
        "identity.closure_email", "identity.deactivation_email",
        "notification.email", "notification.moderator_email", "notification.supporter_email",
        "notification.push", "notification.post_subscribers",
        "instance.notify_announcement", "voice.notify_started",
      ]
    }
  }
```

The plan refuses a split of the types that must stay together. Leave out the types of settings you have not set, as that page
says: a type the server has not registered stops the start, and the plan cannot see which those are. The default service
runs every type that no worker names, and none of those that one does.

A worker also takes the size of its service, each the default service's where it is not named:
[the modules' reference](../../deploy/opentofu/README.md#workers) lists the fields of each platform. A worker takes no
traffic from outside, and an upgrade moves it after the migration and before the default service. Each worker can bring what
the service costs, and the connections of its instances count against the database's limit.

## Changing a backend

Change the `kind` of its section. Each kind sets what it needs on the server, and the plan says what changes. Changing the
database also means moving its data, since a new one is empty: take a [backup](backup.md) of the old one and restore it into
the new one while nothing writes.

### The KEK

Changing `kek.kind` is one line, but on an instance with data it is not free: the database holds secrets wrapped with the
current key, every account's keys among them, so an instance that loses it can no longer read them. Follow
[Move between the local KEK and a KMS](key-management.md#move-between-the-local-kek-and-a-kms). From the secret to a KMS, keep
the current KEK as `kek_previous` while the secrets move: add `secrets = ["kek_previous"]`, and fill the secret
`<name>-kek-previous` with the current KEK. Run `my kek rewrap` through [the console](#the-console), then remove it when
`my kek status` no longer counts `local:` keys. Keep the KEK for as long as you keep database backups from before the rewrap.

On a new instance with no data, nothing needs to move.
