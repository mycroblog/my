# Google Cloud Storage (JSON API)

- Changing an object operation or its preconditions: [Cloud Storage documentation](https://docs.cloud.google.com/storage/docs), search for: JSON API, `ifGenerationMatch`, Request preconditions, Resumable uploads
- Changing a signed URL: [Cloud Storage documentation](https://docs.cloud.google.com/storage/docs), search for: Signed URLs, V4 signing process, `signBlob`, `X-Goog-Expires`
- Calling the client library: [cloud.google.com/go/storage package documentation](https://pkg.go.dev/cloud.google.com/go/storage), search for: `SignedURLOptions`, `SigningSchemeV4`, `ErrObjectNotExist`, `WithJSONReads`
