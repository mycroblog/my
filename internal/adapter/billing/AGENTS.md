# Billing data of the cloud providers

- Changing the Cost Explorer request or reading its answer: [AWS Billing and Cost Management API Reference](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/llms.txt), search for: `GetCostAndUsage`, `RECORD_TYPE`, `NextPageToken`, `Estimated`, `UnblendedCost`
- Changing the query over the Cloud Billing export: [Cloud Billing documentation](https://docs.cloud.google.com/billing/docs), search for: Export Cloud Billing data to BigQuery, `invoice.month`, `usage_start_time`, `credits`
- Calling BigQuery: [BigQuery documentation](https://docs.cloud.google.com/bigquery/docs), search for: `jobs.query`, `jobComplete`, `pageToken`, `useLegacySql`
- Reading a budget notification: [Cloud Billing documentation](https://docs.cloud.google.com/billing/docs), search for: `costAmount`, `budgetAmount`, `costIntervalStart`, `budgetDisplayName`
- Changing the read of Cloudflare's billable usage: [Cloudflare API reference](https://developers.cloudflare.com/api/), search for: Billable Usage, `billing`, `usage`
- Relying on Cloudflare's billing cycles: [Cloudflare Billing documentation](https://developers.cloudflare.com/billing/llms.txt), search for: billing cycle, pay-as-you-go, Invoice
