# S3-compatible object storage

- Changing an object operation or a multipart upload: [Amazon S3 API Reference](https://docs.aws.amazon.com/AmazonS3/latest/API/llms.txt), search for: `PutObject`, `CreateMultipartUpload`, `UploadPart`, `CompleteMultipartUpload`, `AbortMultipartUpload`
- Changing a presigned URL or its expiry: [Amazon S3 User Guide](https://docs.aws.amazon.com/AmazonS3/latest/userguide/llms.txt), search for: presigned URL, expiration, Signature Version 4
- Changing the addressing of a bucket (virtual-hosted or path style): [Amazon S3 User Guide](https://docs.aws.amazon.com/AmazonS3/latest/userguide/llms.txt), search for: Virtual-hosted-style, Path-style requests
- Relying on an operation or a header on Cloudflare R2: [Cloudflare R2 documentation](https://developers.cloudflare.com/r2/llms.txt), search for: S3 API compatibility, `auto`, Presigned URLs
- Relying on an operation or a header on the XML API of Cloud Storage: [Cloud Storage documentation](https://docs.cloud.google.com/storage/docs), search for: XML API, Interoperability, HMAC keys
