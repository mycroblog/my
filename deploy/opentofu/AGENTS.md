# OpenTofu modules for Google Cloud, AWS, and Cloudflare

- Using a language feature (the modules require OpenTofu 1.11 or later): [OpenTofu documentation](https://opentofu.org/docs/), search for: Ephemeral resources, Write-only attributes, `precondition`, `check`, `tofu test`
- Keeping a secret out of the state: [OpenTofu documentation](https://opentofu.org/docs/), search for: Write-only attributes, `ephemeral`, Sensitive Data in State
- Adding or changing a Google Cloud resource (provider 7.x, not the latest major): [OpenTofu Registry, hashicorp/google 7.x](https://search.opentofu.org/provider/hashicorp/google/v7.46.2), search for: `google_cloud_run_v2_service`, `google_secret_manager_secret_version`, `google_cloud_tasks_queue`, `google_compute_backend_service`
- Adding or changing an AWS resource (provider 6.x): [OpenTofu Registry, hashicorp/aws](https://search.opentofu.org/provider/hashicorp/aws/latest), search for: `aws_ecs_service`, `aws_ecs_task_definition`, `aws_secretsmanager_secret_version`, `aws_db_instance`
- Adding or changing a Cloudflare resource (provider 5.x): [OpenTofu Registry, cloudflare/cloudflare](https://search.opentofu.org/provider/cloudflare/cloudflare/latest), search for: `cloudflare_dns_record`, `cloudflare_ruleset`, `cloudflare_tiered_cache`
- Changing how a Cloud Run service scales, starts, or reaches the database: [Cloud Run documentation](https://docs.cloud.google.com/run/docs), search for: Minimum instances, CPU allocation, startup probe, Direct VPC egress
- Changing an ECS service, its task definition, or its deployment: [Amazon ECS Developer Guide](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/llms.txt), search for: task definition, deployment circuit breaker, `awsvpc`, Fargate
