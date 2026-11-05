# blob-aws-s3

The media store: a private S3 bucket that the server reads and serves under `/media/`. It follows
[the backend contract](../../README.md#the-backend-contract).

**This module has never been applied.** The tests mock the provider, so they show that the configuration is consistent, not that AWS accepts it. The server's `s3` adapter is tested against other S3-compatible stores, and this module adds nothing it lacks; the app it needs is what is missing.

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
| `roles` | Reading, writing, and deleting objects, stopping a multipart upload, and listing the bucket |

## On Lambda, an upload does not reach this bucket directly

Every upload goes through the server, and a request to Lambda is at most 6 MB with a binary body in base64: about 4.5 MB of file. The
example lowers `media_max_image_bytes` and `media_max_av_bytes` to fit, which rules out most video. Serving is through the server too, so a
file above 6 MB is sent at 2 MBps. Going beyond these limits needs presigned uploads, which the server does not have, or a CDN in front of `/media/`.

Nothing here has been tried: whether uploads at these sizes work through the adapter, and the time to process one inside a request.
