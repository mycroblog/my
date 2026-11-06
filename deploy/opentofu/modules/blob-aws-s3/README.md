# blob-aws-s3

The media store: a private S3 bucket that the server reads and serves under `/media/`. It follows
[the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. The server's `s3` adapter is tested against other S3-compatible stores, and this module adds nothing it lacks.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "blob" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/blob-aws-s3?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Meaning |
| --- | --- |
| `bucket_name` | Name of the bucket, unique across AWS |

| Output | Value |
| --- | --- |
| `env` | `MY_BLOB`, `MY_BLOB_S3_BUCKET`, `MY_BLOB_S3_REGION` |
| `secret_env` | Empty: no access key, the role is used |
| `permissions` | Reading, writing, and deleting objects, stopping a multipart upload, and listing the bucket |
