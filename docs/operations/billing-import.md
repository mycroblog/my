# Importing cloud usage costs

A guide for operators. The configuration keys are described in the [configuration reference](../reference/configuration.md#billing_sources).

The server imports the usage charges of the providers listed in the setting `billing_sources` every 24 hours and adds them to the instance's costs. The default is empty, which imports nothing.
It imports the charges of the current and the previous billing month (through the 15th of the month, or until the previous month is final), as one line per service and charge type (usage, credit, refund, tax, support, fee, other).
Until the provider closes a month, the values are recorded as estimates; after it closes, as final; and when a value changes after it is final, as a correction. Records of the past are never rewritten.
The state of the imports (the last success, the kind of the last failure, and the number of requests this month) is shown on the costs page of the administration and by `GET /api/v1/admin/billing_imports`.
The metrics count every attempt by source, outcome, and kind of failure (`my.billing.imports`) and report the sources whose newest attempt failed (`my.billing.import.failing`);
[Alerts and runbook](alerts.md#mybillingimportfailing) has the alert on them.

No credentials are kept in this server's configuration. Give the runtime environment what the default credential chain of each provider's SDK reads (for AWS, the execution role, environment variables, or shared credential files; for Google Cloud, Application Default Credentials).

## AWS (`billing_sources = aws`)

The server calls `GetCostAndUsage` of AWS Cost Explorer once per billing month (a UTC calendar month), grouped by service (`SERVICE`) and record type (`RECORD_TYPE`). The endpoint is always `us-east-1`.
Amounts are in USD. While Cost Explorer marks a month as estimated (`Estimated`), the server records the amount up to the previous day as an estimate, and once it no longer does, as final.
A binary built with the build tag `no_billing_aws` refuses `aws` (see [build-tags.md](../reference/build-tags.md)).

### Cost and limits

The Cost Explorer API charges $0.01 for each paginated request. A small account needs one request of one page a day, about $0.31 a month.
To prevent unexpected charges, the server counts requests per UTC day (`billing_aws_max_requests_per_day`, default 4) and per UTC calendar month (`billing_aws_max_requests_per_month`, default 120 = $1.20), and stops without calling once a limit is reached.
Scheduled imports, job retries, and "Import now" in the administration all count. An import that hits a limit is recorded as a failure of kind `request_cap`, and the import of the next day (for the daily limit) or of the first day of the next month (for the monthly limit) tries again.
Before it calls, an import sets aside the requests that the limits leave, so imports that run at the same time cannot exceed the limits together.
Imports of one provider run one at a time: an import that starts while another has not finished tries again a minute later, and a job that already imported successfully calls nothing when it runs again.
An import that stopped before it finished (the process exited) counts the requests it set aside, and the next import waits for it for up to 15 minutes.

### Enable Cost Explorer

1. Open Cost Explorer in the Billing and Cost Management console and enable it. It cannot be disabled once enabled. It takes up to 24 hours for data to be ready.
2. An account where it is not enabled returns `DataUnavailableException`, and the import fails with `not_enabled`.

### IAM permissions

The only permission needed is `ce:GetCostAndUsage` (resource `*`).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    { "Effect": "Allow", "Action": "ce:GetCostAndUsage", "Resource": "*" }
  ]
}
```

How to pass credentials in each runtime environment:

| Runtime | Credentials |
| --- | --- |
| AWS Lambda / ECS | Attach this policy to the execution role (task role). No configuration is needed |
| Google Cloud (Cloud Run) | Create the IAM role of "Calling from Cloud Run with web identity" below and set its ARN in `billing_aws_role_arn`. No AWS keys are needed |
| Your own server | Pass the access key of an IAM user with this policy in `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (or a shared credentials file and `AWS_PROFILE`). Pass it from a secret store |

Missing permissions or invalid credentials fail with `permission`.

### Calling from Cloud Run with web identity

When `billing_aws_role_arn` is set, the server gets a Google ID token of the Cloud Run service account from the metadata server (the audience is the role's ARN) and exchanges it for temporary credentials of that role with `AssumeRoleWithWebIdentity`.

1. In Google Cloud, note the unique ID (the numeric `uniqueId`) of the service account of the Cloud Run service: `gcloud iam service-accounts describe <email> --format='value(uniqueId)'`.
2. In AWS IAM, create a role with the policy of the permissions above, and make its trust policy trust `accounts.google.com`.

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "Federated": "accounts.google.com" },
         "Action": "sts:AssumeRoleWithWebIdentity",
         "Condition": {
           "StringEquals": {
             "accounts.google.com:aud": "<uniqueId of the service account>",
             "accounts.google.com:oaud": "<ARN of this role>"
           }
         }
       }
     ]
   }
   ```

   `accounts.google.com:aud` corresponds to the token's `azp` (the service account's ID), and `accounts.google.com:oaud` to the token's `aud` (the audience the server requests, which is the role's ARN).
3. Set `MY_BILLING_AWS_ROLE_ARN=arn:aws:iam::<account ID>:role/<role name>`.

This setup has not yet been verified on a real account.

### Calling from a linked account of Organizations

The management account of AWS Organizations can see the costs of every member account of the organization. To call with the credentials of a member (linked) account,
turn on "Linked Account Access" in the management account's Cost Explorer preferences. When it is off, the call is refused whatever the IAM permissions are, and the import fails with `not_enabled` or `permission`.

### Choosing which costs to import (filtering)

Choose with `billing_aws_filter`.

| `billing_aws_filter` | Costs imported | Additional settings |
| --- | --- | --- |
| `none` (default) | Every cost the credentials can see | None |
| `linked_account` | The costs of the member accounts you name (`LINKED_ACCOUNT`) | `billing_aws_linked_accounts` (12-digit account IDs, comma-separated) |
| `tag` | The costs of resources that carry the cost allocation tag you name | `billing_aws_tag` (`key=value`, for example `app=my`) |

The most accurate setup is an AWS account dedicated to this server (it may be a member account of Organizations), imported with `none` (calling with that account's credentials) or `linked_account` (calling from the management account).
Taxes, support fees, and some credits cannot carry a tag, and may fall out of a tag filter (not yet verified on a real account).

### Activating a cost allocation tag (`billing_aws_filter = tag`)

A tag on a resource does not appear in the billing data until you activate it as a cost allocation tag.

1. Put the same tag (for example `app=my`) on this server's resources (Lambda functions, RDS, S3 buckets, CloudFront, and so on).
2. In "Cost allocation tags" of the Billing and Cost Management console, select the tag key and activate it. It takes up to 24 hours for a tag to show in this list after you attach it, and up to another 24 hours after activation for it to show in the billing data.
3. To reflect the costs of the period before activation, use "Backfill" on the same screen (the API `StartCostAllocationTagBackfill`).
4. Set `MY_BILLING_AWS_FILTER=tag` and `MY_BILLING_AWS_TAG=app=my`.

### Metric

`billing_aws_metric` is either `NetUnblendedCost` (the default, closer to the invoice), which is after discounts (Savings Plans, reserved instances, negotiated prices), or `UnblendedCost`, which is before them.
Cost Explorer's values and the invoice can differ because of update timing and rounding. When they differ, the invoice is right.

### Kinds of failure

| Kind | AWS response | How to fix |
| --- | --- | --- |
| `permission` | `AccessDeniedException`, invalid credentials | Check the IAM permissions and how the credentials are passed |
| `not_enabled` | `DataUnavailableException` | Enable Cost Explorer. For a linked account, enable Linked Account Access |
| `not_found` | `BillExpirationException` | Cost Explorer no longer has the billing data of the period to import |
| `throttled` | `LimitExceededException` | The job retries after a pause (it counts toward the request limit) |
| `unavailable` | 5xx, network failure | The job retries after a pause (it counts toward the request limit) |
| `invalid_response` | An unreadable response, `ValidationException`, and the like | Check the settings (tag, account IDs) |
| `request_cap` | (no call is made) | Wait for the limiting day or month to pass, or raise the limit |

Retrying cannot fix a failure other than `throttled` and `unavailable`, so the server records it and stops, and the next day's scheduled import tries again.

## Google Cloud (`billing_sources = gcp`)

Google Cloud has no API that returns actual costs, so the server aggregates the Cloud Billing export to BigQuery (standard usage cost) once a day with a BigQuery `jobs.query` (`billing_gcp_method = bigquery`, the default).
When you cannot set up the export, there is also a simple method that imports only an approximate total from the Pub/Sub notifications of a budget (see [Importing from budget notifications](#importing-from-budget-notifications-billing_gcp_method--budget)).
The billing month is the export's `invoice.month`, a calendar month in US Pacific time (with daylight saving time). Dates are recorded in Pacific time as they are.
Amounts are in the billing account's currency, summed as `NUMERIC` per billing month, service (`service.description`), and charge type (`cost_type`), and credits (`credits[]`) become one credit line per service. The export's correction rows that cancel earlier rows are part of the sum.
The export has no mark for finality, so the import that runs from the 5th (Pacific time) of the next month treats the month as final, and until then records it as an estimate from the first day of the month to the day of the last usage. After it is final, the import still reads the month again until the 15th and records any difference as a correction.
A binary built with the build tag `no_billing_gcp` refuses `gcp` (see [build-tags.md](../reference/build-tags.md)).

### Cost and limits

One import is one query, and BigQuery bills the bytes each query scans (at least 10 MB). A small account scans about 0.3 GB a month, which fits the first 1 TiB a month of the free tier (shared with the billing account's other uses), so it costs $0.
To prevent unexpected scans, every query carries `maximumBytesBilled` (`billing_gcp_max_bytes_billed`, default 1 GiB). A query that would exceed it is not billed and fails, and it is recorded as a failure of kind `bytes_cap`. The administration shows the billed bytes.

### Enable the export to BigQuery

The operator sets this up once, in the console (no way to set it through an API has been found).

1. In a project linked to the billing account, create a BigQuery dataset (for example `billing_export`). Choose a multi-region location (`US` or `EU`). The data of the current and the previous month is backfilled (with a single region, the data starts the day you enable it). The location cannot be changed later.
   Do not set an expiration on the tables. Rows that expire do not come back.
2. Under "Billing > Billing export > BigQuery export", enable "Standard usage cost" and select the dataset of step 1. The setup needs "Billing Account Costs Manager" or "Billing Account Administrator" on the billing account and the BigQuery User role on the project.
3. After a few hours to a day, the table `gcp_billing_export_v1_<BILLING_ACCOUNT_ID>` is created (the hyphens of the ID become `_`).
4. Set `MY_BILLING_GCP_TABLE=<project>.billing_export.gcp_billing_export_v1_<BILLING_ACCOUNT_ID>`.

When the dataset or the table cannot be found, the import fails with `not_found`.

### IAM roles

Give the importing server's service account these two roles.

| Role | Where to grant it | Use |
| --- | --- | --- |
| `roles/bigquery.jobUser` (BigQuery Job User) | The project that runs the query (`billing_gcp_query_project`, or the table's project when empty) | `bigquery.jobs.create`. The scanned bytes are billed to this project |
| `roles/bigquery.dataViewer` (BigQuery Data Viewer) | The export's dataset | `bigquery.tables.getData` |

```sh
gcloud projects add-iam-policy-binding <query project> \
  --member=serviceAccount:<service account> --role=roles/bigquery.jobUser
bq add-iam-policy-binding --member=serviceAccount:<service account> \
  --role=roles/bigquery.dataViewer <project>:billing_export
```

When the BigQuery API is not enabled in the query project, the import fails with `not_enabled`, and when a role is missing, with `permission`.

Authentication is Application Default Credentials (ADC).

| Runtime | Credentials |
| --- | --- |
| Google Cloud (Cloud Run) | ADC with the service's service account. No key and no configuration |
| AWS Lambda | Place a credential configuration file of Workload Identity Federation (the AWS type) and pass its path in `GOOGLE_APPLICATION_CREDENTIALS` (not yet verified on a real account) |
| Your own server | Workload Identity Federation, or a service account JSON key passed in `GOOGLE_APPLICATION_CREDENTIALS`. Pass a key from a secret store |

### Choosing which costs to import (filtering)

By default the server imports every cost of the billing account. The most accurate setup is a billing account dedicated to this server, imported without filters.

| Setting | Costs imported |
| --- | --- |
| `billing_gcp_projects` (project IDs, comma-separated) | The costs of the projects you name (`project.id`) |
| `billing_gcp_label` (`key=value`, for example `app=my`) | The costs of resources that carry the label you name (`labels`) |

When you set both, the server imports only the costs that match both.
Costs at the level of the invoice, such as tax, may carry neither a project nor a label, and then fall out of the filter (not yet verified on a real account).

### Kinds of failure

| Kind | BigQuery response | How to fix |
| --- | --- | --- |
| `permission` | 401, 403 (`accessDenied`) | Check the service account's roles and how the credentials are passed |
| `not_enabled` | 403 (`accessNotConfigured` and the like) | Enable the BigQuery API in the query project |
| `not_found` | 404 | Check `billing_gcp_table` and the export setup |
| `bytes_cap` | 400 (`bytesBilledLimitExceeded`) | The scan exceeded `billing_gcp_max_bytes_billed`. Raise the limit if the table is large |
| `throttled` | 429, 403 (`rateLimitExceeded`, `quotaExceeded`) | The job retries after a pause |
| `unavailable` | 5xx, network failure, a query that does not finish | The job retries after a pause |
| `invalid_response` | An unreadable response, rows that mix currencies within a month, any other 400 | Check the export table (is it the standard usage cost?) |

Retrying cannot fix a failure other than `throttled` and `unavailable`, so the server records it and stops, and the next day's scheduled import tries again.

### Importing from budget notifications (`billing_gcp_method = budget`)

This is a simple method for when you cannot set up the BigQuery export. Once a day, the server reads from a pull subscription the notifications that a Cloud Billing budget sends to a Pub/Sub topic.
It uses pull rather than push, so a notification never wakes the server (Cloud Run, for example).

This method imports **only an approximate total based on estimates**.

- A notification's `costAmount` is the estimated cumulative cost of the budget period, with no breakdown by service or charge type. The server imports only one line per month and budget (the service name is the budget's display name, the charge type is usage, and the category is other). Credits and taxes are in the total as the budget is configured.
- As with BigQuery, a month becomes final in the import that runs from the 5th (Pacific time) of the next month. A budget notification knows nothing about the invoice closing, so even a value treated as final can differ from the invoice amount.
- An estimated month is recorded as the cost from the first day of the month to the day (Pacific time) the last notification was published.
- Filtering is decided by the budget's scope (projects, services, one label key and value), and `billing_gcp_projects` and `billing_gcp_label` are not used.

#### Prepare the budget, the topic, and the subscription

1. Create a topic and a pull subscription that only this server reads. See "How notifications are read" below for the reasons behind the retention period and the expiration.

   ```sh
   gcloud pubsub topics create billing-budget --project=<project>
   gcloud pubsub subscriptions create my-billing-budget --project=<project> \
     --topic=billing-budget --ack-deadline=60 \
     --message-retention-duration=31d --expiration-period=never
   ```

2. In the console, under "Billing > Budgets & alerts", create a budget. Make its period **monthly** (a notification of a budget that is not monthly fails with `invalid_response`).
   Select this server's project as the scope, and under "Manage notifications > Connect a Pub/Sub topic to this budget" select the topic of step 1.
   Creating the budget needs "Billing Account Costs Manager" or "Billing Account Administrator" on the billing account, and `pubsub.topics.setIamPolicy` and `pubsub.topics.list` on the topic's project.
   The first notification takes a few hours to arrive, and after that several arrive a day.
3. Give the importing server's service account `roles/pubsub.subscriber` (Pub/Sub Subscriber) on the subscription. It is used to pull messages and acknowledge them (`pubsub.subscriptions.consume`).

   ```sh
   gcloud pubsub subscriptions add-iam-policy-binding my-billing-budget --project=<project> \
     --member=serviceAccount:<service account> --role=roles/pubsub.subscriber
   ```

4. Set `MY_BILLING_GCP_METHOD=budget` and `MY_BILLING_GCP_BUDGET_SUBSCRIPTION=projects/<project>/subscriptions/my-billing-budget`.

Authentication is the same Application Default Credentials as for BigQuery (see the table in [IAM roles](#iam-roles) above).
The build tag `no_billing_gcp` removes this method too. The Pub/Sub client library is shared with `stream = pubsub`, so it leaves the binary only when you also set `no_pubsub` (see [build-tags.md](../reference/build-tags.md)).

#### How notifications are read

- One import repeats Pull until no new message arrives, and takes, for each budget and month, the notification with the latest publish time. This absorbs duplicate Pub/Sub deliveries, repeated notifications with the same value, and reordering, so receiving the same notification many times counts once in the total.
  The number of requests counted for an import is the number of Pulls, normally two: one that returns messages and one that comes back empty. The acknowledgments made at the end (`Acknowledge`, `ModifyAckDeadline`) are not counted.
- The server acknowledges (acks) only a notification that a newer notification of the same budget and month has replaced, and a notification of a month that is no longer imported (the previous month, once it has been recorded as final). The latest notification of an imported month is not acked but returned to the subscription, so the next import reads the same notification again, and the value does not change on a day with no new notification.
- Therefore, set the subscription's message retention to **at least 14 days (31 days recommended)**. The last notification of a month's budget arrives around the 2nd of the next month, and the previous month is read again until the 15th. A month whose notifications have expired is recorded as a cost of 0.
- By default a subscription expires after 31 days without use. Set the expiration to "never" so that it survives a period in which imports were stopped.
- Do not send other messages to the topic. A message that cannot be read as a budget notification fails that import with `invalid_response`, and is acked so that the next import does not stall on it.
- Cost: a few dozen messages a day fit the Pub/Sub free tier (10 GiB a month), so it costs $0. The server sets no limit.

#### Kinds of failure (budget notifications)

| Kind | Pub/Sub response | How to fix |
| --- | --- | --- |
| `permission` | `PERMISSION_DENIED`, `UNAUTHENTICATED` | Check `roles/pubsub.subscriber` on the subscription and how the credentials are passed |
| `not_enabled` | `PERMISSION_DENIED` (`SERVICE_DISABLED`) | Enable the Pub/Sub API in the subscription's project |
| `not_found` | `NOT_FOUND` | Check `billing_gcp_budget_subscription` and whether the subscription has expired |
| `throttled` | `RESOURCE_EXHAUSTED` | The job retries after a pause |
| `unavailable` | `UNAVAILABLE`, `INTERNAL`, and the like, network failure | The job retries after a pause |
| `invalid_response` | A message that cannot be read as a budget notification, a budget that is not monthly, two budgets with the same display name | Make the budget monthly, give the budgets distinct display names, and send no other messages to the topic |
