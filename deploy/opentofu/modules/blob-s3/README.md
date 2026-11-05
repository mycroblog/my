# blob-s3

The media store on any S3-compatible service, given an endpoint and a key. It creates nothing and follows
[the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** It has no provider and its tests only check the settings it returns. The server's `s3`
adapter is tested on its own against S3-compatible stores; whether your store accepts it is what stays unverified.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "blob" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/blob-s3?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `endpoint` | required | Origin of the S3 API. Use `https` unless the server is on a private network you trust |
| `bucket` | required | The bucket, which exists already |
| `access_key_id_secret_id`, `secret_access_key_secret_id` | required | Secrets that hold the key: an ID in Secret Manager, an ARN in AWS Secrets Manager |
| `region` | `null` | Region of the bucket. Unset, the server signs for `us-east-1`, which most S3-compatible servers expect |
| `path_style` | `false` | `true` for a server with no host name per bucket, such as MinIO |

| Output | Value |
| --- | --- |
| `env` | `MY_BLOB`, `MY_BLOB_S3_BUCKET`, `MY_BLOB_S3_ENDPOINT`, and `MY_BLOB_S3_REGION` and `MY_BLOB_S3_PATH_STYLE` when they are set |
| `secret_env` | `MY_BLOB_S3_ACCESS_KEY_ID`, `MY_BLOB_S3_SECRET_ACCESS_KEY` |
| `permissions` | Empty: the app module grants access to every secret it is given |

Create both secrets and add their values by hand before the first apply, so that no value reaches the state. The key ID is not
secret to the server, but it goes through the store with the key so that the two are replaced together.

Keep the bucket private. The server reads the files from it and serves them under `/media/`, so nothing needs to reach the bucket
directly. For Cloudflare R2 use [`blob-cloudflare-r2`](../blob-cloudflare-r2/README.md), which also creates the bucket. For Amazon
S3 use [`blob-aws-s3`](../blob-aws-s3/README.md), which needs no key.
