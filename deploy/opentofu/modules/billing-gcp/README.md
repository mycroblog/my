# billing-gcp

The Google Cloud side of the usage cost import, `billing_sources = gcp`, by either `billing_gcp_method`: the dataset of the
Cloud Billing export to BigQuery, or the topic and the pull subscription of a budget's notifications, each with the roles the
server needs. It follows [the backend contract](../../README.md#the-backend-contract).
[Importing cloud usage costs](../../../../docs/operations/billing-import.md) describes the import itself.

**This module has never been applied, and the import through it has never run on a real account.** The tests mock the
provider, so they show that the configuration is consistent, not that Google Cloud accepts it. Unverified:

- that a budget connects to this topic and publishes to it, and that the export writes to this dataset, with the module's grants in place;
- that `READER` in the dataset's access entry shows no difference on the next plan, and that the entry leaves Google's own entry for the export in place;
- on a server outside Google Cloud, everything past the grants: read [Outside Google Cloud](#outside-google-cloud).

## Source

Pin it to the release you deploy, like every module of the deployment. It needs the `google` provider, also on an app of
another platform:

```hcl
module "billing_gcp" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/billing-gcp?ref=v0.1.0&depth=1"

  project_id         = var.project_id
  billing_account_id = "012345-6789AB-CDEF01"
}

# In backends of the app:
module.billing_gcp,
```

| Input | Default | Meaning |
| --- | --- | --- |
| `project_id` | required | Project of the dataset, or of the topic and the subscription |
| `method` | `bigquery` | `bigquery`, or `budget` |
| `billing_account_id` | required with `bigquery` | The billing account, which names the export's table |
| `dataset_id` | `billing_export` | The dataset of the export |
| `create_dataset` | `true` | Off for an export that already has a dataset |
| `dataset_location` | `US` | A multi-region receives the current and the previous month when the export starts, and a single region starts the day it is enabled. It cannot change later |
| `query_project` | `project_id` | The project that runs the queries and is billed for them. Set `MY_BILLING_GCP_QUERY_PROJECT` this way, not through `env` |
| `name` | `my-billing-budget` | The topic and the subscription, read by this server alone |
| `external_member` | null | For a server outside Google Cloud: read below |

| Output | Value |
| --- | --- |
| `billing_sources` | `["gcp"]`, which the app adds to `MY_BILLING_SOURCES` |
| `env` | `MY_BILLING_GCP_METHOD`, and `MY_BILLING_GCP_TABLE` (with `MY_BILLING_GCP_QUERY_PROJECT` when set) or `MY_BILLING_GCP_BUDGET_SUBSCRIPTION` |
| `secret_env` | Empty |
| `permissions` | `bigquery`: `roles/bigquery.jobUser` on the query project and `READER` on the dataset, which is what the API makes of `roles/bigquery.dataViewer`. `budget`: `roles/pubsub.subscriber` on the subscription. Empty with `external_member` |
| `budget_topic` | The topic to connect the budget to, or null. This output is not part of the backend contract |

The filters and the limit (`MY_BILLING_GCP_PROJECTS`, `MY_BILLING_GCP_LABEL`, `MY_BILLING_GCP_MAX_BYTES_BILLED`) go through `env` of the app.

The subscription is pull, with an acknowledgment deadline of 60 seconds, 31 days of retention, and no expiration, which is
what the import needs: the guide says why. Nothing calls the server, so this module has no `invokers`. Send nothing else to
the topic, and let nothing else read the subscription.

The API of the service must be on in the project: `bigquery.googleapis.com` where the queries run, or `pubsub.googleapis.com`.
This module does not turn it on, like the other modules of the deployment.

A dataset that holds tables refuses to be destroyed. That is on purpose: expired or deleted rows of the export do not come back.

## What stays manual

Apply first, so that the dataset or the topic exists, then do these once in the Console. Neither has a way through this
module, and each needs a role on the billing account, which the identity that runs `tofu` does not otherwise need.

| Method | In the Console | Roles it needs |
| --- | --- | --- |
| `bigquery` | Under Billing > Billing export > BigQuery export, enable "Standard usage cost" and select the dataset. The table appears after a few hours to a day, and until then an import fails with `not_found` | Billing Account Costs Manager or Billing Account Administrator on the billing account, and BigQuery User on the project |
| `budget` | Under Billing > Budgets & alerts, create a **monthly** budget and connect the topic of the output `budget_topic` to it. The first notification takes a few hours, and until then there is nothing to import | Billing Account Costs Manager or Billing Account Administrator on the billing account, and permission to grant the Pub/Sub Publisher role on the topic (`pubsub.topics.setIamPolicy`, `pubsub.topics.list`) |

Connecting the budget lets Cloud Billing publish, and enabling the export adds Google's service account to the dataset, so
this module grants neither and manages no policy that would remove them.

## Outside Google Cloud

The app of another platform refuses a Google Cloud role, so with `external_member` this module makes the grants itself and
returns no `permissions`. The value must be known at plan time, so build it from names rather than from the attributes of a
resource.

```hcl
external_member = "principalSet://iam.googleapis.com/projects/<number>/locations/global/workloadIdentityPools/<pool>/attribute.aws_role/<role>"
```

The rest is yours, and **a server on AWS cannot use this yet**: the pool and its provider are not part of this module, and
the server finds Application Default Credentials through `GOOGLE_APPLICATION_CREDENTIALS`, the path of a file that AWS's app
modules have no way to deliver. The file holds no secret, but it would have to be in the image.
