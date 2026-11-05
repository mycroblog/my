# blob-gcp-gcs

The media store: a private Cloud Storage bucket, with uniform access and public access prevention. It follows
[the backend contract](../../README.md#the-backend-contract).

| Input | Meaning |
| --- | --- |
| `project_id` | Project of the bucket |
| `bucket_name` | Name, unique across Google Cloud |
| `location` | Location. The Always Free storage covers `us-east1`, `us-west1`, and `us-central1` only |

| Output | Value |
| --- | --- |
| `env` | `MY_BLOB`, `MY_BLOB_GCS_BUCKET` |
| `secret_env` | Empty |
| `roles` | `roles/storage.objectUser` on the bucket |

The server reads the bucket and serves the media under `/media/`. It signs no URLs at present, so the service account does
not get `roles/iam.serviceAccountTokenCreator` on itself; a backend of your own that needs signed URLs would add it.

Back the bucket up with the database. Deleting it fails while it holds objects.
