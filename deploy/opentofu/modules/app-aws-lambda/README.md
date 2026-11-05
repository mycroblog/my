# app-aws-lambda

The server on AWS Lambda: a web function behind a Function URL, a worker function that a queue invokes, and one-off functions
for the migration and the console. It follows [the app contract](../../README.md#the-app-contract).

**This module has never been applied, and it cannot run yet.** The tests mock the provider. It was written to find out whether the
contract of these modules holds on a second platform. What it lacks:

- **The server does not read a secret from Secrets Manager.** Lambda has no way to resolve a secret into a variable or mount one, so
  this module sets `SECRET_ARN_<NAME>` to its ARN, for example `SECRET_ARN_MY_DATABASE_URL`. No version of `my` reads those, so a
  function fails at start for lack of `database_url` and `kek`.
- **No image runs here.** The released image has no adapter, and is multi-architecture, which Lambda refuses. See [The image](#the-image).
- **No one-off command exists.** The migration and console functions run `one_off_command`, by default `lambda-run`, which `my` does
  not have. It must take `{"args":[...]}` as an invocation and answer `{"exit_code":N,"output":"..."}`, which is what `console.sh`
  and `upgrade.sh` send and read. So nothing here can create the schema yet.

## Source

Pin it to the release you deploy, like every module of the deployment:

```hcl
module "app" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/modules/app-aws-lambda?ref=v0.1.0&depth=1"
  # ...
}
```

| Input | Default | Meaning |
| --- | --- | --- |
| `image` | required | Image in Amazon ECR, in the function's region |
| `architecture` | `x86_64` | Architecture of that image |
| `host` | required | Public domain |
| `name` | `my` | Start of the names |
| `profile` | `production` | `profile` of the server |
| `backends`, `env`, `secret_env`, `secret_files` | empty | [The backend contract](../../README.md#the-backend-contract). `secret_files` is treated as `secret_env`. The module sets `MY_BILLING_SOURCES` from what `backends` returns as `billing_sources` |
| `web_memory_mib`, `web_timeout_seconds` | `512`, `900` | The web function. The timeout is the longest a stream stays open |
| `web_reserved_concurrency` | null | Most instances of the web function, which bounds the database connections. An account must keep 100 unreserved |
| `worker_memory_mib`, `worker_timeout_seconds` | `1024`, `900` | The worker function. From 330 seconds |
| `worker_batch_size`, `worker_max_concurrency` | `10`, `5` | How the queue invokes it |
| `job_memory_mib`, `job_timeout_seconds` | `1024`, `900` | The one-off functions |
| `one_off_command` | `["lambda-run"]` | What they run. Assumed: read above |
| `console_enabled` | `false` | Creates the console function |
| `network` | `null` | Attach every function to a network, for a backend that lives in one: a network from [`network-aws-vpc`](../network-aws-vpc/README.md), or an object of [its shape](../../README.md#the-network-on-aws). Every function takes its private subnets and a group of the module's that allows the way out, beside those of `security_group_ids` of the backends. An attached function reaches the internet and AWS's APIs only through what the network provides, and a public subnet does not help, so a network with `nat` off fails the plan |
| `log_retention_days` | `30` | CloudWatch Logs |

| Output | Value |
| --- | --- |
| `service_name`, `worker_function`, `migration_function` | The functions |
| `function_url`, `origin_host` | The Function URL and its host name |
| `role_arn` | Execution role of every function |
| `vpc_subnet_ids` | The subnets that every function attaches to, or empty |
| `console` | What `console.sh` reads, or null |
| `env`, `secret_env`, `secret_references` | The merged settings |
| `one_off_secret_env` | The secret settings of the one-off functions: `secret_env` with `direct_secret_env` over it |

## The image

The module assumes an image that you build and push to a repository in ECR, in the function's region, before the first apply. It
does not create the repository: a function cannot be created before its image exists, so a repository in the same apply fails.
Whoever applies needs `ecr:BatchGetImage`, `ecr:GetDownloadUrlForLayer`, `ecr:GetRepositoryPolicy`, and `ecr:SetRepositoryPolicy` on it,
with which Lambda adds what it needs to the repository's policy.

```dockerfile
FROM ghcr.io/mycroblog/my:0.1.0
COPY --from=public.ecr.aws/awsguru/aws-lambda-adapter:1.1.0 /lambda-adapter /opt/extensions/lambda-adapter
```

Build one platform, with no attestation, so that the result is a single image rather than an index:

```sh
docker buildx build --platform linux/amd64 --provenance=false --sbom=false -t <ECR image> --push .
```

**This has not been built or run.** Whether the adapter, running as an extension, works from a distroless image; whether the user
Lambda runs as can read everything in the image; and whether the adapter gets out of the way of `one_off_command` are unknown.

## What differs from Cloud Run

| | Cloud Run | Here |
| --- | --- | --- |
| Processes | One service with `role = all` | Two functions, `web` and `worker`, because the adapter has one invoke mode per function: streams need `response_stream`, the queue's events `buffered` |
| Requests per instance | 80 | One. An open stream holds a whole instance for up to 15 minutes, and each instance opens its own database pool, so set `pool_max_conns` low |
| Request body | 32 MiB | 6 MB for the event, and a binary body is base64 inside it, so about 4.5 MB of file |
| Response | Up to the timeout | 200 MB when streamed, limited to 2 MBps after the first 6 MB. The worker's answers are 6 MB at most |
| A bad image | Takes no traffic until `/readyz` answers | Nothing holds it back: requests that start an instance on it fail |
| One-off commands | A job, an hour by default | 15 minutes, and a step that takes longer cannot run here |
| All settings | No limit that matters | 4 KB of environment variables per function, which the plan checks once the values are known |
| IAM | A binding per kind of resource | One inline policy, up to 10,240 characters, which the plan checks |
| Who may reach it | An explicit `service_ingress`, and a load balancer can be the only way in | Everyone can connect, and the server refuses what lacks the header that CloudFront adds. A Function URL has `NONE` or `AWS_IAM`, and `AWS_IAM` with an origin access control needs the SHA-256 of every `POST` body from the client, which a peer does not send. There is one value that works, so an input would be a choice in name only |

## Unverified

- Everything against a real account.
- The client address. The connection comes from the adapter on loopback, so `127.0.0.1/32` is trusted and `X-Forwarded-For` decides. A direct caller of the URL is refused by the origin secret, so it cannot forge an address through it, but the shape of the header through CloudFront is unverified.
- A stream through `response_stream` on a Function URL, and whether a process frozen between invocations keeps its SNS listener. The server is
  written to recover when its queue was swept away while frozen.
- Several `Set-Cookie` headers through the adapter and a Function URL.
- How long `my serve` takes to answer `/readyz` at a cold start against what Lambda allows the init phase, with the secrets read first.
- Anything `my serve` writes outside `/tmp`: the file system of a function is read-only.
