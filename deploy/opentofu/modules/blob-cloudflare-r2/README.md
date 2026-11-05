# blob-cloudflare-r2

The media store on Cloudflare R2: a private bucket, and the settings of the server's `s3` adapter. It follows
[the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that
Cloudflare accepts it. Unverified: creating the bucket with the provider, the credentials below against the bucket, and uploads
and multipart uploads through the server's adapter to a real R2 bucket.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "blob" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/blob-cloudflare-r2?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `account_id`, `bucket_name` | required | The Cloudflare account and the bucket to create |
| `access_key_id_secret_id`, `secret_access_key_secret_id` | required | Secrets that hold the S3 credentials: an ID in Secret Manager, an ARN in AWS Secrets Manager |
| `location` | `null` | Location hint: best effort, and honoured only when the name is first created |
| `jurisdiction` | `default` | `default`, `eu`, `fedramp`, or `us`. It decides the endpoint |

| Output | Value |
| --- | --- |
| `env` | `MY_BLOB`, `MY_BLOB_S3_BUCKET`, `MY_BLOB_S3_ENDPOINT`, `MY_BLOB_S3_REGION` (`auto`) |
| `secret_env` | `MY_BLOB_S3_ACCESS_KEY_ID`, `MY_BLOB_S3_SECRET_ACCESS_KEY` |
| `permissions` | Empty: the app module grants access to every secret it is given |

The `cloudflare` provider needs a token that may create R2 buckets. That token is not the one the server uses.

## The credentials are yours to create

The provider can create an API token (`cloudflare_account_token`), and R2's S3 credentials derive from a token: the Access Key ID
is the token's `id`, and the Secret Access Key is the SHA-256 of its `value`. This module does not do that, because `value` is a
computed attribute with no write-only form and no ephemeral resource, **so the secret would land in the state**. Keep it out:

1. In the Cloudflare dashboard, on the R2 overview page, select **Manage** next to **API Tokens**, and create an account token with
   **Object Read & Write** on this bucket alone. It shows the Secret Access Key once.
2. Put the two values in the platform's secret store, and pass the two references here.

The bucket has to exist for the token to name it, so run the first apply with `-target=module.blob`, then create the token, then
apply the rest. The app reads `latest`, so to replace the token, add new versions and start a new revision.

## Why the bucket is not public

R2 and [`ingress-cloudflare-media`](../ingress-cloudflare-media/README.md) are separate things. The server reads the files from the
private bucket and serves them under `/media/`, and that is what a CDN caches: **the server stays the CDN's origin**, as
[Put a CDN in front of media delivery](../../../../docs/operations/cdn.md) says. So this module creates no custom domain and no
managed domain, either of which would serve the bucket to anyone. The two can sit in the same deployment and share nothing.

## Cost

As of 2026-11-05 on [R2's pricing page](https://developers.cloudflare.com/r2/pricing/): 10 GB-month of Standard storage, 1 million
Class A and 10 million Class B operations free each month, then $0.015 per GB-month, $4.50 and $0.36 per million operations.
Egress is free. Nothing is billed while the server is idle beyond what is stored, so it fits a deployment that scales to zero.

R2 expires an unfinished multipart upload seven days after it starts by default, so this module adds no lifecycle rule, where
`blob-aws-s3` must.
