# billing-aws

The AWS side of the usage cost import, `billing_sources = aws`: the Cost Explorer permission for a server on AWS, or the role
that a server on Cloud Run assumes with its Google ID token. It follows [the backend contract](../../README.md#the-backend-contract).
[Importing cloud usage costs](../../../../docs/operations/billing-import.md) describes the import itself.

**This module has never been applied, and the import through it has never run on a real account.** The tests mock the
provider, so they show that the configuration is consistent, not that AWS accepts it. Unverified:

- that AWS accepts the trust policy, and that the token of a Cloud Run service account meets its conditions;
- that the server on Cloud Run gets credentials from it and reads Cost Explorer;
- what an import returns on a real account.

## Source

Pin it to the release you deploy, like every module of the deployment. The block is the same on any AWS app, and needs the
`aws` provider, which that app has already:

```hcl
module "billing_aws" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/billing-aws?ref=v0.1.0&depth=1"
}

# In backends of the app:
module.billing_aws,
```

## On Cloud Run

This is the one module of AWS that `app-gcp-cloud-run` takes, and only with `web_identity = true`, since reading another cloud's costs is only an outside API call: [Platforms](../../README.md#platforms).

The role is on AWS and the server on Google Cloud, so the root configuration needs both providers, and credentials for both
when it plans. The module itself uses only `aws`: it takes the service account's ID as a string. A deployment on Google Cloud
without this block needs no `aws` provider.

```hcl
provider "aws" {
  region = "us-east-1" # IAM is global, but the provider asks for a region
}

module "billing_aws" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/billing-aws?ref=v0.1.0&depth=1"

  web_identity                     = true
  google_service_account_unique_id = module.app.service_account_unique_id
}

# In backends of the app. Name the outputs, not the whole module: a reference to the whole module waits for the role, which
# waits for the app's service account, which waits for the backends, and that is a cycle.
{
  platform        = module.billing_aws.platform
  env             = module.billing_aws.env
  permissions     = module.billing_aws.permissions
  billing_sources = module.billing_aws.billing_sources
},
```

The role's ARN comes from its name and the caller's account, so the service can be created before the role exists. Until the
role exists an import fails with `permission`, and the next day's import tries again.

| Input | Default | Meaning |
| --- | --- | --- |
| `web_identity` | `false` | Create the role for a server on Google Cloud. Off, the app's own identity gets the permission |
| `google_service_account_unique_id` | required with `web_identity` | The numeric ID of the server's service account, the only identity that may assume the role |
| `role_name` | `my-billing-import` | Name of the role, used by this instance alone |

| Output | Value |
| --- | --- |
| `billing_sources` | `["aws"]`, which the app adds to `MY_BILLING_SOURCES` |
| `env` | `MY_BILLING_AWS_ROLE_ARN` with `web_identity`, otherwise empty |
| `secret_env` | Empty |
| `permissions` | `ce:GetCostAndUsage` on `*`, or empty with `web_identity`, where the role holds it |
| `role_arn` | The role's ARN, or null. This output is not part of the backend contract |

The filter and the limits (`MY_BILLING_AWS_FILTER`, `MY_BILLING_AWS_MAX_REQUESTS_PER_DAY`, and the rest) go through `env` of the app.

## What stays manual

These need the account's Billing and Cost Management console, and this module does none of them:

- enable Cost Explorer, once per account, which an operator with access to the billing console does; it cannot be disabled, and data takes up to 24 hours;
- for a linked account, turn on Linked Account Access in the management account's Cost Explorer preferences;
- for a tag filter, activate the cost allocation tag.

A server outside both clouds uses an IAM user's keys rather than this module: read the guide above.
