# SPDX-License-Identifier: AGPL-3.0-only

# The contract of the backends, on a mocked provider: nothing here reaches Google Cloud.

mock_provider "google" {
  mock_resource "google_service_account" {
    defaults = {
      email  = "my-server@p.iam.gserviceaccount.com"
      member = "serviceAccount:my-server@p.iam.gserviceaccount.com"
    }
  }
}

variables {
  service_ingress = "all"

  project_id   = "p"
  region       = "asia-northeast1"
  image        = "ghcr.io/mycroblog/my:0.1.0"
  host         = "social.example.org"
  secret_files = { MY_KEK_FILE = "kek" }
  backends = [
    {
      env        = { MY_DATABASE = "postgres" }
      secret_env = { MY_DATABASE_URL = "db-url" }
    },
    {
      env      = { MY_QUEUE = "cloudtasks" }
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
    env        = { MY_MAIL = "smtp" }
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
        env        = { MY_DATABASE = "sqlite" }
        secret_env = { MY_DATABASE_URL = "other-url" }
      },
      {
        env      = { MY_QUEUE = "cloudtasks" }
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
      { env = { MY_DATABASE = "postgres" } },
      { env = { MY_DATABASE = "sqlite" } },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_in_env_and_secret_env_fails" {
  command = plan

  variables {
    env        = { MY_VAPID_PRIVATE_KEY = "plain" }
    secret_env = { MY_VAPID_PRIVATE_KEY = "vapid" }
  }

  expect_failures = [google_service_account.server]
}

run "a_setting_the_module_owns_fails" {
  command = plan

  variables {
    env = { MY_HOST = "other.example.org" }
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
      { permissions = [{ permission = "roles/x", scope = "somewhere", resource = "r" }] },
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
    env          = { MY_KEY_MANAGER = "gcpkms" }
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
      { secret_files = run.kek_in_secret_manager.secret_files },
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
      { env = run.kek_in_kms_made_elsewhere.env, permissions = run.kek_in_kms_made_elsewhere.permissions },
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
      { service_ingress = "all" },
      { service_ingress = "internal-and-load-balancer" },
    ]
  }

  expect_failures = [google_service_account.server]
}

run "an_unknown_ingress_setting_fails" {
  command = plan

  variables {
    backends = [{ service_ingress = "OPEN" }]
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
      { env = run.ingress_domain_mapping.env, service_ingress = run.ingress_domain_mapping.service_ingress, trusted_proxies = run.ingress_domain_mapping.trusted_proxies },
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
      { env = run.ingress_load_balancer.env, service_ingress = run.ingress_load_balancer.service_ingress, trusted_proxies = run.ingress_load_balancer.trusted_proxies },
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
    backends = [{ env = { MY_MEDIA_ORIGIN = "https://media.example.org" } }]
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
      { env = run.ingress_domain_mapping.env, service_ingress = run.ingress_domain_mapping.service_ingress, trusted_proxies = run.ingress_domain_mapping.trusted_proxies },
      { env = { MY_MEDIA_ORIGIN = "https://media.example.org" } },
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
      { env = run.ingress_load_balancer.env, service_ingress = run.ingress_load_balancer.service_ingress, trusted_proxies = run.ingress_load_balancer.trusted_proxies },
      { env = { MY_MEDIA_ORIGIN = "https://media.example.org" } },
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
      { trusted_proxies = run.ingress_domain_mapping.trusted_proxies },
      { trusted_proxies = run.ingress_load_balancer.trusted_proxies },
    ]
  }

  expect_failures = [google_service_account.server]
}

# A backend of another platform fails rather than being dropped by the conversion to this module's type.
run "a_backend_for_aws_fails" {
  command = plan

  variables {
    backends = [
      { permissions = [{ permission = "sqs:SendMessage", scope = "policy", resource = "arn:aws:sqs:us-east-1:111122223333:my-jobs" }] },
    ]
  }

  expect_failures = [var.backends]
}

run "the_default_url_stays_on_and_jobs_must_name_this_service" {
  command = plan

  variables {
    env = { MY_QUEUE_CLOUDTASKS_URL = "https://my-123456789012.asia-northeast1.run.app" }
  }

  assert {
    condition     = google_cloud_run_v2_service.server.default_uri_disabled == false
    error_message = "Cloud Tasks and Cloud Scheduler call the default URL"
  }
}

run "jobs_that_call_another_service_fail" {
  command = plan

  variables {
    env = { MY_QUEUE_CLOUDTASKS_URL = "https://other-123456789012.asia-northeast1.run.app" }
  }

  expect_failures = [google_service_account.server]
}

run "jobs_that_call_another_region_fail" {
  command = plan

  variables {
    env = { MY_QUEUE_CLOUDTASKS_URL = "https://my-123456789012.us-central1.run.app" }
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
    backends = [{ service_ingress = "internal-and-load-balancer" }]
  }

  expect_failures = [google_service_account.server]
}

run "a_load_balancer_needs_its_setting" {
  command = plan

  variables {
    service_ingress = "internal-and-load-balancer"
    backends        = [{ service_ingress = "all" }]
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
    backends = [{ direct_secret_env = { MY_DATABASE_URL = "direct" } }]
  }

  expect_failures = [google_service_account.server]
}

run "a_cloud_sql_backend_mounts_the_socket_on_the_service_and_the_jobs" {
  command = plan

  variables {
    backends = [{ cloud_sql_instances = ["p:r:db"] }]
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
