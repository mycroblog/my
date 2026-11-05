# SPDX-License-Identifier: AGPL-3.0-only

# The contract of the backends, on mocked providers: nothing here reaches a cloud.

mock_provider "aws" {
  mock_resource "aws_s3_bucket" {
    defaults = {
      arn = "arn:aws:s3:::media"
    }
  }
}

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
}

variables {
  env = { MY_QUEUE = "cloudtasks" }

  service_ingress = "all"

  project_id   = "p"
  region       = "asia-northeast1"
  image        = "ghcr.io/mycroblog/my:0.1.0"
  host         = "social.example.org"
  secret_files = { MY_KEK_FILE = "kek" }
  backends = [
    {
      platform   = "gcp"
      env        = { MY_DATABASE = "postgres" }
      secret_env = { MY_DATABASE_URL = "db-url" }
    },
    {
      platform = "gcp"
      invokers = ["invoker@p.iam.gserviceaccount.com"]
      permissions = [
        { permission = "roles/cloudtasks.enqueuer", scope = "queue", resource = "projects/p/locations/l/queues/q" },
      ]
    },
  ]
}

run "merges_every_source" {
  command = plan

  variables {
    env        = { MY_QUEUE = "cloudtasks", MY_MAIL = "smtp" }
    secret_env = { MY_VAPID_PRIVATE_KEY = "vapid" }
  }

  assert {
    condition = output.env == {
      MY_DATABASE        = "postgres"
      MY_HOST            = "social.example.org"
      MY_KEK_FILE        = "/secrets/my-kek-file/value"
      MY_MAIL            = "smtp"
      MY_PROFILE         = "production"
      MY_QUEUE           = "cloudtasks"
      MY_ROLE            = "all"
      MY_TRUSTED_PROXIES = "169.254.0.0/16"
    }
    error_message = "env is not the merge of what the module sets, the backends, and the env input"
  }

  assert {
    condition     = output.secret_env == tomap({ MY_DATABASE_URL = "db-url", MY_VAPID_PRIVATE_KEY = "vapid" })
    error_message = "secret_env is not the merge of the backends and the secret_env input"
  }

  assert {
    condition     = toset(keys(google_secret_manager_secret_iam_member.server)) == toset(["db-url", "kek", "vapid"])
    error_message = "the service account must read every secret it is given"
  }

  assert {
    condition     = length(google_cloud_tasks_queue_iam_member.backend) == 1 && length(google_project_iam_member.backend) == 0
    error_message = "a backend's permission must go on the scope it names"
  }

  assert {
    condition     = keys(google_cloud_run_v2_service_iam_member.invoker) == ["invoker@p.iam.gserviceaccount.com"]
    error_message = "each invoker must get run.invoker on the service"
  }
}

run "swapping_a_backend_changes_only_its_settings" {
  command = plan

  variables {
    backends = [
      {
        platform   = "gcp"
        env        = { MY_DATABASE = "sqlite" }
        secret_env = { MY_DATABASE_URL = "other-url" }
      },
      {
        platform = "gcp"
        invokers = ["invoker@p.iam.gserviceaccount.com"]
        permissions = [
          { permission = "roles/cloudtasks.enqueuer", scope = "queue", resource = "projects/p/locations/l/queues/q" },
        ]
      },
    ]
  }

  assert {
    condition     = output.env["MY_DATABASE"] == "sqlite" && output.secret_env["MY_DATABASE_URL"] == "other-url"
    error_message = "the settings of the swapped backend did not change"
  }

  assert {
    condition     = output.env["MY_QUEUE"] == "cloudtasks" && output.env["MY_HOST"] == "social.example.org" && length(google_cloud_tasks_queue_iam_member.backend) == 1
    error_message = "swapping one backend changed something else"
  }
}

run "a_setting_named_by_two_backends_fails" {
  command = plan

  variables {
    backends = [
      { platform = "gcp", env = { MY_DATABASE = "postgres" } },
      { platform = "gcp", env = { MY_DATABASE = "sqlite" } },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_in_env_and_secret_env_fails" {
  command = plan

  variables {
    env        = { MY_QUEUE = "cloudtasks", MY_VAPID_PRIVATE_KEY = "plain" }
    secret_env = { MY_VAPID_PRIVATE_KEY = "vapid" }
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_the_module_owns_fails" {
  command = plan

  variables {
    env = { MY_QUEUE = "cloudtasks", MY_HOST = "other.example.org" }
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_and_its_file_form_fail" {
  command = plan

  variables {
    secret_env = { MY_KEK = "another-kek" }
  }

  expect_failures = [google_service_account.server]
}

run "a_permission_without_its_scope_fails" {
  command = plan

  variables {
    backends = [
      { platform = "gcp", permissions = [{ permission = "roles/x", scope = "somewhere", resource = "r" }] },
    ]
  }

  expect_failures = [var.backends]
}

run "latest_is_refused" {
  command = plan

  variables {
    image = "ghcr.io/mycroblog/my:latest"
  }

  expect_failures = [var.image]
}

run "console_is_off_by_default" {
  command = plan

  assert {
    condition     = keys(google_cloud_run_v2_job.one_off) == ["migrate"] && output.console == null
    error_message = "only the migration job may exist while the console is off"
  }
}

run "console_on" {
  command = plan

  variables {
    console_enabled = true
  }

  assert {
    condition     = toset(keys(google_cloud_run_v2_job.one_off)) == toset(["migrate", "console"])
    error_message = "the console job is missing"
  }

  assert {
    condition     = output.console.platform == "gcp-cloud-run" && output.console.project == "p" && output.console.region == "asia-northeast1"
    error_message = "the console output does not say how to run it"
  }
}

run "no_kek_fails" {
  command = plan

  variables {
    secret_files = {}
  }

  expect_failures = [google_service_account.server]
}

run "a_kms_without_a_key_fails" {
  command = plan

  variables {
    secret_files = {}
    env          = { MY_QUEUE = "cloudtasks", MY_KEY_MANAGER = "gcpkms" }
  }

  expect_failures = [google_service_account.server]
}

run "a_secret_file_needs_a_file_name" {
  command = plan

  variables {
    secret_files = { MY_KEK = "kek" }
  }

  expect_failures = [var.secret_files]
}

# Swapping the KEK: the same app with each kek-* module, in one line of backends.
run "kek_generated_in_secret_manager" {
  command = plan

  module {
    source = "../kek-gcp-secret-manager"
  }

  variables {
    project_id = "p"
  }

  assert {
    condition     = google_secret_manager_secret_version.kek.secret_data_wo_version == 1 && google_secret_manager_secret_version.kek.deletion_policy == "ABANDON"
    error_message = "the version of the generated KEK must stay fixed, and survive leaving the configuration"
  }
}

# Applied, for run.* to read its outputs below; nothing with prevent_destroy is created.
run "kek_in_secret_manager" {
  module {
    source = "../kek-gcp-secret-manager"
  }

  variables {
    project_id = "p"
    create     = false
  }

  assert {
    condition     = google_secret_manager_secret_version.kek == null
    error_message = "a secret made elsewhere must get no version from this module"
  }
}

run "kek_in_kms" {
  command = plan

  module {
    source = "../kek-gcp-kms"
  }

  variables {
    project_id = "p"
    location   = "asia-northeast1"
  }

  assert {
    condition     = google_kms_crypto_key.kek != null
    error_message = "the module must create the key"
  }
}

# Applied, for run.* to read its outputs below; nothing with prevent_destroy is created.
run "kek_in_kms_made_elsewhere" {
  module {
    source = "../kek-gcp-kms"
  }

  variables {
    project_id    = "p"
    location      = "asia-northeast1"
    crypto_key_id = "projects/p/locations/l/keyRings/r/cryptoKeys/other"
  }

  assert {
    condition     = google_kms_crypto_key.kek == null && output.env["MY_KMS_KEY"] == "projects/p/locations/l/keyRings/r/cryptoKeys/other"
    error_message = "a key made elsewhere must be used, and none created"
  }
}

run "app_with_the_secret_manager_kek" {
  command = plan

  variables {
    secret_files = {}
    backends = [
      { platform = run.kek_in_secret_manager.platform, secret_files = run.kek_in_secret_manager.secret_files },
    ]
  }

  assert {
    condition     = output.env["MY_KEK_FILE"] == "/secrets/my-kek-file/value" && !contains(keys(output.env), "MY_KEY_MANAGER")
    error_message = "the KEK must arrive as a mounted file"
  }

  assert {
    condition     = keys(google_secret_manager_secret_iam_member.server) == ["my-kek"] && length(google_kms_crypto_key_iam_member.backend) == 0
    error_message = "the service account must read the secret and hold no role on a key"
  }

  assert {
    condition     = one(google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].volumes).secret[0].secret == "my-kek"
    error_message = "the jobs must mount the KEK as the service does"
  }
}

run "app_with_the_kms_kek" {
  command = plan

  variables {
    secret_files    = {}
    console_enabled = true
    backends = [
      { platform = run.kek_in_kms_made_elsewhere.platform, env = run.kek_in_kms_made_elsewhere.env, permissions = run.kek_in_kms_made_elsewhere.permissions },
    ]
  }

  assert {
    condition     = output.env["MY_KEY_MANAGER"] == "gcpkms" && output.env["MY_KMS_KEY"] == "projects/p/locations/l/keyRings/r/cryptoKeys/other" && !contains(keys(output.env), "MY_KEK_FILE")
    error_message = "the KMS settings must replace the mounted file"
  }

  assert {
    condition     = keys(google_kms_crypto_key_iam_member.backend) == ["projects/p/locations/l/keyRings/r/cryptoKeys/other/roles/cloudkms.cryptoKeyEncrypterDecrypter"] && length(google_project_iam_member.backend) == 0
    error_message = "the grant must be on the key, not on the project"
  }

  assert {
    condition     = length(google_secret_manager_secret_iam_member.server) == 0
    error_message = "the KMS needs no secret"
  }

  assert {
    condition     = google_cloud_run_v2_job.one_off["console"].template[0].template[0].service_account == google_service_account.server.email && google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].service_account == google_service_account.server.email
    error_message = "the jobs must run as the account that holds the grant"
  }
}

run "no_ingress" {
  command = plan

  assert {
    condition     = google_cloud_run_v2_service.server.ingress == "INGRESS_TRAFFIC_ALL" && output.env["MY_TRUSTED_PROXIES"] == "169.254.0.0/16"
    error_message = "without an ingress the service must be open, and trust Cloud Run's front end"
  }
}

run "two_ingresses_asking_differently_fail" {
  command = plan

  variables {
    backends = [
      { platform = "gcp", service_ingress = "all" },
      { platform = "gcp", service_ingress = "internal-and-load-balancer" },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "an_unknown_ingress_setting_fails" {
  command = plan

  variables {
    backends = [{ platform = "gcp", service_ingress = "OPEN" }]
  }

  expect_failures = [var.backends]
}

# Swapping the ingress: the same app with each ingress-* module.
run "ingress_domain_mapping" {
  module {
    source = "../ingress-gcp-domain-mapping"
  }

  variables {
    project_id   = "p"
    region       = "asia-northeast1"
    host         = "social.example.org"
    service_name = "my"
  }

  assert {
    condition     = google_cloud_run_domain_mapping.host.spec[0].route_name == "my"
    error_message = "the mapping must point at the service"
  }
}

run "ingress_load_balancer" {
  module {
    source = "../ingress-gcp-load-balancer"
  }

  variables {
    project_id   = "p"
    region       = "asia-northeast1"
    host         = "social.example.org"
    service_name = "my"
  }

  assert {
    condition     = google_compute_region_network_endpoint_group.service.cloud_run[0].service == "my" && google_compute_managed_ssl_certificate.host.managed[0].domains[0] == "social.example.org"
    error_message = "the load balancer must reach the service, and hold a certificate for host"
  }

  assert {
    condition     = google_compute_global_forwarding_rule.https.port_range == "443" && google_compute_global_forwarding_rule.redirect.port_range == "80" && google_compute_url_map.redirect.default_url_redirect[0].https_redirect
    error_message = "the load balancer must serve HTTPS and redirect HTTP"
  }

  assert {
    condition     = one(output.dns_records).type == "A" && one(output.dns_records).rrdata == output.ip_address
    error_message = "the DNS record must point at the load balancer"
  }
}

run "app_with_the_domain_mapping" {
  command = plan

  variables {
    backends = [
      { platform = run.ingress_domain_mapping.platform, env = run.ingress_domain_mapping.env, service_ingress = run.ingress_domain_mapping.service_ingress, trusted_proxies = run.ingress_domain_mapping.trusted_proxies },
    ]
  }

  assert {
    condition     = google_cloud_run_v2_service.server.ingress == "INGRESS_TRAFFIC_ALL" && output.env["MY_TRUSTED_PROXIES"] == "169.254.0.0/16"
    error_message = "the mapping must leave the service open, and trust Cloud Run's front end"
  }
}

run "app_with_the_load_balancer" {
  command = plan

  variables {
    service_ingress = "internal-and-load-balancer"
    backends = [
      { platform = run.ingress_load_balancer.platform, env = run.ingress_load_balancer.env, service_ingress = run.ingress_load_balancer.service_ingress, trusted_proxies = run.ingress_load_balancer.trusted_proxies },
    ]
  }

  assert {
    condition     = google_cloud_run_v2_service.server.ingress == "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
    error_message = "with a load balancer the service must take traffic from it alone"
  }

  assert {
    condition     = startswith(output.env["MY_TRUSTED_PROXIES"], "169.254.0.0/16, ") && length(split(", ", output.env["MY_TRUSTED_PROXIES"])) == 2
    error_message = "the server must trust the load balancer's address to find the client's"
  }

  assert {
    condition     = output.env["MY_HOST"] == "social.example.org" && output.env["MY_KEK_FILE"] == "/secrets/my-kek-file/value"
    error_message = "swapping the ingress changed something else"
  }
}

# A media host beside each ingress: no setting is named twice, since the media module sets no trust.
run "ingress_media" {
  command = plan

  module {
    source = "../ingress-cloudflare-media"
  }

  variables {
    zone_id     = "0123456789abcdef0123456789abcdef"
    host        = "media.example.org"
    origin_host = "ghs.googlehosted.com"
  }
}

run "media_alone" {
  command = plan

  variables {
    backends = [{ platform = "gcp", env = { MY_MEDIA_ORIGIN = "https://media.example.org" } }]
  }

  assert {
    condition     = output.env["MY_MEDIA_ORIGIN"] == "https://media.example.org" && output.env["MY_TRUSTED_PROXIES"] == "169.254.0.0/16"
    error_message = "alone, the media host must leave the trust at Cloud Run's front end"
  }
}

run "media_beside_the_domain_mapping" {
  command = plan

  variables {
    backends = [
      { platform = run.ingress_domain_mapping.platform, env = run.ingress_domain_mapping.env, service_ingress = run.ingress_domain_mapping.service_ingress, trusted_proxies = run.ingress_domain_mapping.trusted_proxies },
      { platform = "gcp", env = { MY_MEDIA_ORIGIN = "https://media.example.org" } },
    ]
  }

  assert {
    condition     = output.env["MY_MEDIA_ORIGIN"] == "https://media.example.org" && output.env["MY_TRUSTED_PROXIES"] == "169.254.0.0/16"
    error_message = "the media host must sit beside the mapping"
  }
}

run "media_beside_the_load_balancer" {
  command = plan

  variables {
    service_ingress = "internal-and-load-balancer"
    backends = [
      { platform = run.ingress_load_balancer.platform, env = run.ingress_load_balancer.env, service_ingress = run.ingress_load_balancer.service_ingress, trusted_proxies = run.ingress_load_balancer.trusted_proxies },
      { platform = "gcp", env = { MY_MEDIA_ORIGIN = "https://media.example.org" } },
    ]
  }

  assert {
    condition     = output.env["MY_MEDIA_ORIGIN"] == "https://media.example.org" && google_cloud_run_v2_service.server.ingress == "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
    error_message = "the media host must sit beside the load balancer"
  }
}

run "two_ingresses_that_set_trust_fail" {
  command = plan

  variables {
    backends = [
      { platform = run.ingress_domain_mapping.platform, trusted_proxies = run.ingress_domain_mapping.trusted_proxies },
      { platform = run.ingress_load_balancer.platform, trusted_proxies = run.ingress_load_balancer.trusted_proxies },
    ]
  }

  expect_failures = [google_service_account.server]
}

# A backend of another platform fails rather than being dropped by the conversion to this module's type.
run "the_bucket_of_aws" {
  command = apply

  module {
    source = "../blob-aws-s3"
  }

  variables {
    bucket_name = "media"
  }

  assert {
    condition     = output.platform == "aws"
    error_message = "the module must say that it belongs to AWS"
  }
}

run "a_module_of_aws_fails" {
  command = plan

  variables {
    backends = [run.the_bucket_of_aws]
  }

  expect_failures = [var.backends]
}

run "a_backend_of_aws_by_platform_alone_fails" {
  command = plan

  variables {
    backends = [{ platform = "aws", env = { MY_MAIL = "ses" } }]
  }

  expect_failures = [var.backends]
}

run "a_backend_of_no_known_platform_fails" {
  command = plan

  variables {
    backends = [{ platform = "azure" }]
  }

  expect_failures = [var.backends]
}

# The one exception, stated in the validation of backends.
run "billing_aws_with_its_own_role_passes" {
  command = plan

  variables {
    backends = [{
      platform        = "aws"
      env             = { MY_BILLING_AWS_ROLE_ARN = "arn:aws:iam::111122223333:role/my-billing-import" }
      billing_sources = ["aws"]
    }]
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws"
    error_message = "reading AWS's costs from Cloud Run is the one module of AWS that this app takes"
  }
}

run "billing_aws_without_web_identity_fails" {
  command = plan

  variables {
    backends = [{
      platform        = "aws"
      permissions     = [{ permission = "ce:GetCostAndUsage", resource = "*" }]
      billing_sources = ["aws"]
    }]
  }

  expect_failures = [var.backends]
}

run "the_queue_in_the_database" {
  command = apply

  module {
    source = "../queue-db"
  }
}

# The queue in the database needs both settings: one keeps an instance running, and the other gives it CPU outside requests.
run "the_queue_in_the_database_fails_by_default" {
  command = plan

  variables {
    env      = {}
    backends = [run.the_queue_in_the_database]
  }

  expect_failures = [google_service_account.server]
}

run "the_queue_in_the_database_fails_with_min_instances_alone" {
  command = plan

  variables {
    env           = {}
    backends      = [run.the_queue_in_the_database]
    min_instances = 1
  }

  expect_failures = [google_service_account.server]
}

run "the_queue_in_the_database_fails_with_always_allocated_cpu_alone" {
  command = plan

  variables {
    env                  = {}
    backends             = [run.the_queue_in_the_database]
    cpu_always_allocated = true
  }

  expect_failures = [google_service_account.server]
}

run "the_queue_in_the_database_passes_with_both" {
  command = plan

  variables {
    env                  = {}
    backends             = [run.the_queue_in_the_database]
    min_instances        = 1
    cpu_always_allocated = true
  }

  assert {
    condition     = output.env["MY_QUEUE"] == "db"
    error_message = "an instance kept running with its CPU can poll the table"
  }

  assert {
    condition = (
      google_cloud_run_v2_service.server.template[0].scaling[0].min_instance_count == 1 &&
      google_cloud_run_v2_service.server.template[0].containers[0].resources[0].cpu_idle == false
    )
    error_message = "the two inputs must reach the service"
  }
}

run "the_service_scales_to_zero_with_cpu_during_requests_by_default" {
  command = plan

  assert {
    condition = (
      google_cloud_run_v2_service.server.template[0].scaling[0].min_instance_count == 0 &&
      google_cloud_run_v2_service.server.template[0].containers[0].resources[0].cpu_idle == true
    )
    error_message = "the defaults must stay what a deployment with queue = cloudtasks needs"
  }
}

run "min_instances_above_the_maximum_fail" {
  command = plan

  variables {
    min_instances = 4
  }

  expect_failures = [var.min_instances]
}

run "no_queue_is_the_queue_in_the_database_and_fails" {
  command = plan

  variables {
    env      = {}
    backends = []
  }

  expect_failures = [google_service_account.server]
}

run "no_queue_passes_with_both" {
  command = plan

  variables {
    env                  = {}
    backends             = []
    min_instances        = 1
    cpu_always_allocated = true
  }

  assert {
    condition     = !contains(keys(output.env), "MY_QUEUE")
    error_message = "the server's default is the queue in the database, so the two settings cover it too"
  }
}

run "the_default_url_stays_on_and_jobs_must_name_this_service" {
  command = plan

  variables {
    env = { MY_QUEUE = "cloudtasks", MY_QUEUE_CLOUDTASKS_URL = "https://my-123456789012.asia-northeast1.run.app" }
  }

  assert {
    condition     = google_cloud_run_v2_service.server.default_uri_disabled == false
    error_message = "Cloud Tasks and Cloud Scheduler call the default URL"
  }
}

run "jobs_that_call_another_service_fail" {
  command = plan

  variables {
    env = { MY_QUEUE = "cloudtasks", MY_QUEUE_CLOUDTASKS_URL = "https://other-123456789012.asia-northeast1.run.app" }
  }

  expect_failures = [google_service_account.server]
}

run "jobs_that_call_another_region_fail" {
  command = plan

  variables {
    env = { MY_QUEUE = "cloudtasks", MY_QUEUE_CLOUDTASKS_URL = "https://my-123456789012.us-central1.run.app" }
  }

  expect_failures = [google_service_account.server]
}

run "the_setting_is_the_inputs" {
  command = plan

  variables {
    service_ingress = "internal"
  }

  assert {
    condition     = google_cloud_run_v2_service.server.ingress == "INGRESS_TRAFFIC_INTERNAL_ONLY" && output.service_ingress == "internal"
    error_message = "a deployment with no ingress chooses for itself"
  }
}

run "an_ingress_that_disagrees_fails" {
  command = plan

  variables {
    backends = [{ platform = "gcp", service_ingress = "internal-and-load-balancer" }]
  }

  expect_failures = [google_service_account.server]
}

run "a_load_balancer_needs_its_setting" {
  command = plan

  variables {
    service_ingress = "internal-and-load-balancer"
    backends        = [{ platform = "gcp", service_ingress = "all" }]
  }

  expect_failures = [google_service_account.server]
}

run "an_unknown_setting_fails" {
  command = plan

  variables {
    service_ingress = "open"
  }

  expect_failures = [var.service_ingress]
}

run "the_jobs_take_the_direct_connection_and_the_service_the_pooled_one" {
  command = plan

  variables {
    backends = [{
      platform          = "gcp"
      env               = { MY_DATABASE = "postgres" }
      secret_env        = { MY_DATABASE_URL = "pooled", MY_STREAM_POSTGRES_URL = "direct" }
      direct_secret_env = { MY_DATABASE_URL = "direct" }
    }]
  }

  assert {
    condition     = [for e in google_cloud_run_v2_service.server.template[0].containers[0].env : e.value_source[0].secret_key_ref[0].secret if e.name == "MY_DATABASE_URL"] == ["pooled"]
    error_message = "the service must use the pooled connection"
  }

  assert {
    condition     = [for e in google_cloud_run_v2_service.server.template[0].containers[0].env : e.value_source[0].secret_key_ref[0].secret if e.name == "MY_STREAM_POSTGRES_URL"] == ["direct"]
    error_message = "LISTEN must use the direct connection"
  }

  assert {
    condition     = [for e in google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].containers[0].env : e.value_source[0].secret_key_ref[0].secret if e.name == "MY_DATABASE_URL"] == ["direct"]
    error_message = "the migration job must use the direct connection"
  }

  assert {
    condition     = contains(keys(google_secret_manager_secret_iam_member.server), "pooled") && contains(keys(google_secret_manager_secret_iam_member.server), "direct")
    error_message = "the app must be able to read both secrets"
  }
}

run "the_console_takes_the_direct_connection_too" {
  command = plan

  variables {
    console_enabled = true
    backends = [{
      platform          = "gcp"
      secret_env        = { MY_DATABASE_URL = "pooled" }
      direct_secret_env = { MY_DATABASE_URL = "direct" }
    }]
  }

  assert {
    condition     = [for e in google_cloud_run_v2_job.one_off["console"].template[0].template[0].containers[0].env : e.value_source[0].secret_key_ref[0].secret if e.name == "MY_DATABASE_URL"] == ["direct"]
    error_message = "the console must use the direct connection"
  }
}

run "without_a_pooler_the_jobs_and_the_service_are_the_same" {
  command = plan

  assert {
    condition     = output.one_off_secret_env == output.secret_env
    error_message = "with no direct_secret_env, the jobs take what the service takes"
  }
}

run "a_direct_setting_with_nothing_to_override_fails" {
  command = plan

  variables {
    backends = [{ platform = "gcp", direct_secret_env = { MY_DATABASE_URL = "direct" } }]
  }

  expect_failures = [google_service_account.server]
}

run "a_cloud_sql_backend_mounts_the_socket_on_the_service_and_the_jobs" {
  command = plan

  variables {
    backends = [{ platform = "gcp", cloud_sql_instances = ["p:r:db"] }]
  }

  assert {
    condition     = [for v in google_cloud_run_v2_service.server.template[0].volumes : tolist(v.cloud_sql_instance[0].instances)[0] if v.name == "cloudsql"] == ["p:r:db"]
    error_message = "the service must have the instance as a volume"
  }

  assert {
    condition     = [for m in google_cloud_run_v2_service.server.template[0].containers[0].volume_mounts : m.mount_path if m.name == "cloudsql"] == ["/cloudsql"]
    error_message = "the service must mount the socket at /cloudsql"
  }

  assert {
    condition     = [for v in google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].volumes : v.cloud_sql_instance[0].instances[0] if v.name == "cloudsql"] == ["p:r:db"]
    error_message = "the migration job must have the instance as a volume"
  }

  assert {
    condition     = [for m in google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].containers[0].volume_mounts : m.mount_path if m.name == "cloudsql"] == ["/cloudsql"]
    error_message = "the migration job must mount the socket at /cloudsql"
  }
}

run "without_cloud_sql_there_is_no_volume" {
  command = plan

  assert {
    condition     = length([for v in google_cloud_run_v2_service.server.template[0].volumes : v if v.name == "cloudsql"]) == 0
    error_message = "a deployment without Cloud SQL must not change"
  }
}

run "no_vpc_by_default" {
  command = plan

  assert {
    condition     = length(google_cloud_run_v2_service.server.template[0].vpc_access) == 0 && length(google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].vpc_access) == 0
    error_message = "a deployment that asks for no VPC must attach none"
  }
}

run "the_network_reaches_the_service_and_the_jobs" {
  command = plan

  variables {
    console_enabled = true
    network         = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s" }
  }

  assert {
    condition = alltrue([
      for t in [
        google_cloud_run_v2_service.server.template[0],
        google_cloud_run_v2_job.one_off["migrate"].template[0].template[0],
        google_cloud_run_v2_job.one_off["console"].template[0].template[0],
      ] : t.vpc_access[0].egress == "PRIVATE_RANGES_ONLY" && t.vpc_access[0].network_interfaces[0].subnetwork == "projects/p/regions/asia-northeast1/subnetworks/s"
    ])
    error_message = "the service and every job must take the subnet, for private ranges only"
  }
}

# These modules have no resources, so applying them reaches nothing, and run.* can read their outputs below.
run "kv_external" {
  command = apply

  module {
    source = "../kv-external"
  }

  variables {
    url_secret_id    = "valkey-url"
    tls_ca_secret_id = "valkey-ca"
  }
}

run "stream_beside_the_kv" {
  command = apply

  module {
    source = "../stream-valkey"
  }

  variables {
    url_secret_id = null
  }
}

run "kv_and_stream_share_one_url" {
  command = plan

  variables {
    backends = [
      { platform = run.kv_external.platform, env = run.kv_external.env, secret_env = run.kv_external.secret_env, secret_files = run.kv_external.secret_files },
      { platform = run.stream_beside_the_kv.platform, env = run.stream_beside_the_kv.env, secret_env = run.stream_beside_the_kv.secret_env, secret_files = run.stream_beside_the_kv.secret_files },
    ]
  }

  assert {
    condition     = output.env["MY_KV"] == "valkey" && output.env["MY_STREAM"] == "valkey" && output.secret_env["MY_VALKEY_URL"] == "valkey-url"
    error_message = "the one URL must serve both"
  }

  assert {
    condition     = output.env["MY_VALKEY_TLS_CA_FILE"] == "/secrets/my-valkey-tls-ca-file/value" && contains(keys(google_secret_manager_secret_iam_member.server), "valkey-ca")
    error_message = "the CA must arrive as a mounted file that the service account can read"
  }
}

# Replaces the run above of the same module, whose outputs nothing reads after this.
run "stream_with_its_own_url" {
  command = apply

  module {
    source = "../stream-valkey"
  }

  variables {
    url_secret_id = "valkey-url"
  }
}

run "kv_and_stream_both_naming_the_url_fail" {
  command = plan

  variables {
    backends = [
      { platform = run.kv_external.platform, env = run.kv_external.env, secret_env = run.kv_external.secret_env },
      { platform = run.stream_with_its_own_url.platform, env = run.stream_with_its_own_url.env, secret_env = run.stream_with_its_own_url.secret_env },
    ]
  }

  expect_failures = [google_service_account.server]
}

# Several backends add to the one setting, which is absent while none does.
run "billing_sources_are_absent_without_a_billing_backend" {
  command = plan

  assert {
    condition     = !contains(keys(output.env), "MY_BILLING_SOURCES")
    error_message = "a deployment without a billing backend must not name the setting"
  }
}

run "two_backends_add_to_billing_sources" {
  command = plan

  variables {
    backends = [
      { platform = "gcp", env = { MY_DATABASE = "postgres" }, secret_env = { MY_DATABASE_URL = "db-url" } },
      { platform = "gcp", billing_sources = ["gcp"], env = { MY_BILLING_GCP_METHOD = "budget" } },
      { platform = "gcp", billing_sources = ["aws"] },
    ]
  }

  assert {
    condition     = output.env["MY_BILLING_SOURCES"] == "aws,gcp"
    error_message = "the setting must list each source of the backends"
  }
}

run "a_billing_source_named_twice_fails" {
  command = plan

  variables {
    backends = [
      { platform = "gcp", env = { MY_DATABASE = "postgres" }, secret_env = { MY_DATABASE_URL = "db-url" } },
      { platform = "gcp", billing_sources = ["aws"] },
      { platform = "gcp", billing_sources = ["aws"] },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "billing_sources_beside_the_setting_in_env_fail" {
  command = plan

  variables {
    env = { MY_QUEUE = "cloudtasks", MY_BILLING_SOURCES = "aws" }
    backends = [
      { platform = "gcp", env = { MY_DATABASE = "postgres" }, secret_env = { MY_DATABASE_URL = "db-url" } },
      { platform = "gcp", billing_sources = ["gcp"] },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "the_scopes_of_billing_grant_on_their_resources" {
  command = plan

  variables {
    backends = [
      {
        platform = "gcp"
        permissions = [
          { permission = "roles/bigquery.jobUser", scope = "project", resource = "other" },
          { permission = "READER", scope = "dataset", resource = "projects/billing/datasets/export" },
          { permission = "roles/pubsub.subscriber", scope = "subscription", resource = "projects/billing/subscriptions/budget" },
        ]
      },
    ]
  }

  assert {
    condition     = google_project_iam_member.backend["other/roles/bigquery.jobUser"].project == "other"
    error_message = "a project the backend names must get the grant, not the app's"
  }

  assert {
    condition = (
      google_bigquery_dataset_access.backend["projects/billing/datasets/export/READER"].project == "billing" &&
      google_bigquery_dataset_access.backend["projects/billing/datasets/export/READER"].dataset_id == "export" &&
      google_bigquery_dataset_access.backend["projects/billing/datasets/export/READER"].user_by_email == "my-server@p.iam.gserviceaccount.com"
    )
    error_message = "the dataset's access entry must name the dataset and the service account"
  }

  assert {
    condition = (
      google_pubsub_subscription_iam_member.backend["projects/billing/subscriptions/budget/roles/pubsub.subscriber"].project == "billing" &&
      google_pubsub_subscription_iam_member.backend["projects/billing/subscriptions/budget/roles/pubsub.subscriber"].subscription == "budget"
    )
    error_message = "the subscription's binding must name the subscription"
  }
}

run "a_project_without_a_resource_is_still_the_apps" {
  command = plan

  variables {
    backends = [
      { platform = "gcp", permissions = [{ permission = "roles/x", scope = "project" }] },
    ]
  }

  assert {
    condition     = google_project_iam_member.backend["roles/x"].project == "p"
    error_message = "a project grant without a resource goes on the app's project, under the key it always had"
  }
}

run "the_security_groups_of_aws_fail" {
  command = plan

  variables {
    backends = [{ platform = "gcp", security_group_ids = ["sg-1"] }]
  }

  expect_failures = [var.backends]
}

run "a_queue_that_lambda_polls_fails" {
  command = plan

  variables {
    backends = [{ platform = "gcp", event_sources = { jobs = { arn = "a", visibility_timeout_seconds = 930 } } }]
  }

  expect_failures = [var.backends]
}

run "the_load_balancer_of_ecs_fails" {
  command = plan

  variables {
    backends = [{ platform = "gcp", load_balancer = { target_group_arn = "a", security_group_id = "b" } }]
  }

  expect_failures = [var.backends]
}

run "the_conditions_of_an_aws_statement_fail" {
  command = plan

  variables {
    backends = [{ platform = "gcp", permissions = [{ permission = "roles/x", scope = "project", conditions = [{ test = "t", variable = "v", values = ["x"] }] }] }]
  }

  expect_failures = [var.backends]
}

run "a_backend_on_the_same_network_passes" {
  command = plan

  variables {
    network  = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s" }
    backends = [{ platform = "gcp", private_network = "projects/p/global/networks/n" }]
  }

  assert {
    condition     = length(google_cloud_run_v2_service.server.template[0].vpc_access) == 1
    error_message = "the service must attach to the network"
  }
}

run "a_backend_on_private_addresses_with_no_network_fails" {
  command = plan

  variables {
    backends = [{ platform = "gcp", private_network = "projects/p/global/networks/n" }]
  }

  expect_failures = [google_service_account.server]
}

run "a_backend_on_another_network_fails" {
  command = plan

  variables {
    network  = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s" }
    backends = [{ platform = "gcp", private_network = "projects/p/global/networks/other" }]
  }

  expect_failures = [google_service_account.server]
}

run "a_network_of_another_platform_fails" {
  command = plan

  variables {
    network = { platform = "aws", vpc_id = "vpc-1" }
  }

  expect_failures = [var.network]
}

run "a_subnet_of_another_region_fails" {
  command = plan

  variables {
    network = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/elsewhere/subnetworks/s" }
  }

  expect_failures = [var.network]
}

run "stream_postgres_on_a_database_url" {
  command = apply

  module {
    source = "../stream-postgres"
  }
}

run "db_with_a_pooler" {
  command = apply

  module {
    source = "../db-external"
  }

  variables {
    url_secret_id        = "direct"
    pooled_url_secret_id = "pooled"
  }
}

run "a_pooled_database_names_the_listeners_url_and_stream_postgres_names_nothing" {
  command = plan

  variables {
    backends = [
      { platform = run.db_with_a_pooler.platform, env = run.db_with_a_pooler.env, secret_env = run.db_with_a_pooler.secret_env, direct_secret_env = run.db_with_a_pooler.direct_secret_env },
      { platform = run.stream_postgres_on_a_database_url.platform, env = run.stream_postgres_on_a_database_url.env, secret_env = run.stream_postgres_on_a_database_url.secret_env },
    ]
  }

  assert {
    condition     = output.env["MY_STREAM"] == "postgres" && output.secret_env["MY_STREAM_POSTGRES_URL"] == "direct" && output.secret_env["MY_DATABASE_URL"] == "pooled"
    error_message = "LISTEN must use the direct connection, which the database module names"
  }
}

run "stream_postgres_with_its_own_url" {
  command = apply

  module {
    source = "../stream-postgres"
  }

  variables {
    listen_url_secret_id = "other"
  }
}

run "two_owners_of_the_listeners_url_fail" {
  command = plan

  variables {
    backends = [
      { platform = run.db_with_a_pooler.platform, env = run.db_with_a_pooler.env, secret_env = run.db_with_a_pooler.secret_env, direct_secret_env = run.db_with_a_pooler.direct_secret_env },
      { platform = run.stream_postgres_with_its_own_url.platform, env = run.stream_postgres_with_its_own_url.env, secret_env = run.stream_postgres_with_its_own_url.secret_env },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "the_guard_of_outbound_http_stays_on_a_network" {
  command = plan

  variables {
    network = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s" }
    env     = { MY_HTTP_ALLOW_PRIVATE_NETWORKS = "true" }
  }

  expect_failures = [google_service_account.server]
}

run "egress_for_private_ranges_needs_no_nat" {
  command = plan

  variables {
    network = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s", nat = false }
  }

  assert {
    condition     = google_cloud_run_v2_service.server.template[0].vpc_access[0].egress == "PRIVATE_RANGES_ONLY"
    error_message = "the default must keep the service's own path to the internet"
  }
}

run "all_traffic_with_no_nat_fails" {
  command = plan

  variables {
    vpc_egress = "ALL_TRAFFIC"
    network    = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s", nat = false }
  }

  expect_failures = [var.vpc_egress]
}

run "all_traffic_with_no_network_fails" {
  command = plan

  variables {
    vpc_egress = "ALL_TRAFFIC"
  }

  expect_failures = [var.vpc_egress]
}

run "all_traffic_with_nat_passes" {
  command = plan

  variables {
    vpc_egress = "ALL_TRAFFIC"
    network    = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s", nat = true }
  }

  assert {
    condition     = google_cloud_run_v2_service.server.template[0].vpc_access[0].egress == "ALL_TRAFFIC" && google_cloud_run_v2_job.one_off["migrate"].template[0].template[0].vpc_access[0].egress == "ALL_TRAFFIC"
    error_message = "the service and the jobs must take the setting"
  }
}

run "all_traffic_with_the_connector_of_cloud_sql_fails" {
  command = plan

  variables {
    vpc_egress = "ALL_TRAFFIC"
    network    = { platform = "gcp", network = "projects/p/global/networks/n", subnetwork = "projects/p/regions/asia-northeast1/subnetworks/s", nat = true }
    backends   = [{ platform = "gcp", cloud_sql_instances = ["p:r:i"] }]
  }

  expect_failures = [google_service_account.server]
}
