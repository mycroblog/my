# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

locals {
  named_settings = concat(
    flatten([for b in var.backends : concat(keys(b.env), keys(b.secret_env), keys(b.secret_files))]),
    keys(var.env),
    keys(var.secret_env),
    keys(var.secret_files),
  )
  # Cloud Run's own front end connects to the server, and an ingress says what connects to Cloud Run. MY_TRUSTED_PROXIES in
  # env replaces the value only without an ingress: with one, the setting is named twice and fails.
  ingress_trust = [for b in var.backends : b.trusted_proxies if b.trusted_proxies != null]
  base_env = merge(
    {
      MY_HOST    = var.host
      MY_ROLE    = var.role
      MY_PROFILE = var.profile
    },
    contains(local.named_settings, "MY_TRUSTED_PROXIES") && length(local.ingress_trust) == 0 ? {} : {
      MY_TRUSTED_PROXIES = join(", ", concat(["169.254.0.0/16"], flatten(local.ingress_trust)))
    },
  )

  # Cloud Run's settings by a plain name. An ingress says which it needs, and the plan fails below when that is not this one.
  cloud_run_ingress = {
    all                        = "INGRESS_TRAFFIC_ALL"
    internal                   = "INGRESS_TRAFFIC_INTERNAL_ONLY"
    internal-and-load-balancer = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"
  }
  ingress_disagreements = distinct([for b in var.backends : b.service_ingress if b.service_ingress != null && b.service_ingress != var.service_ingress])

  # Each one is mounted at a directory of its own, and its variable names the path.
  secret_files = merge(var.secret_files, [for b in var.backends : b.secret_files]...)
  file_mounts = {
    for name, secret in local.secret_files : name => {
      secret = secret
      volume = lower(replace(name, "_", "-"))
      path   = "/secrets/${lower(replace(name, "_", "-"))}"
    }
  }
  file_env = { for name, mount in local.file_mounts : name => "${mount.path}/value" }

  # Every setting by the name the server reads: MY_KEK and MY_KEK_FILE are one setting.
  setting_names = [
    for name in concat(["GOMEMLIMIT"], keys(local.base_env), local.named_settings) : trimsuffix(name, "_FILE")
  ]
  duplicate_settings = [
    for name in distinct(local.setting_names) : name
    if length([for other in local.setting_names : other if other == name]) > 1
  ]

  # merge() alone would let the last one win without a word; the precondition below refuses that first.
  env        = merge(local.base_env, local.file_env, var.env, [for b in var.backends : b.env]...)
  secret_env = merge(var.secret_env, [for b in var.backends : b.secret_env]...)

  secret_ids = toset(concat(values(local.secret_env), values(local.secret_files)))

  jobs_url = lookup(local.env, "MY_QUEUE_CLOUDTASKS_URL", "")

  # The server refuses to start without these, but only once it runs: say it at plan time.
  key_manager = lookup(local.env, "MY_KEY_MANAGER", "local")
  kek_missing = (
    local.key_manager == "local" ? !contains(local.setting_names, "MY_KEK") : !contains(local.setting_names, "MY_KMS_KEY")
  )

  permissions = flatten([for b in var.backends : b.permissions])
  invokers    = toset(flatten([for b in var.backends : b.invokers]))

  jobs = merge(
    { migrate = ["db", "upgrade", "-check"] },
    var.console_enabled ? { console = ["version"] } : {},
  )
}

resource "google_service_account" "server" {
  project      = var.project_id
  account_id   = "${var.name}-server"
  display_name = "The server and its one-off jobs"

  # Here because this resource always exists, and the plan fails before anything is created.
  lifecycle {
    precondition {
      condition     = length(local.duplicate_settings) == 0
      error_message = "These settings are named more than once across the backends, env, secret_env, secret_files, and what this module sets itself: ${join(", ", local.duplicate_settings)}."
    }
    precondition {
      condition     = length(local.ingress_disagreements) == 0
      error_message = "service_ingress is ${var.service_ingress}, but an ingress module needs ${join(" and ", local.ingress_disagreements)}. A load balancer reaches the service only with internal-and-load-balancer, and it should be the only way in; a domain mapping and Cloudflare come from the internet and need all. Set service_ingress to what the ingress needs."
    }
    precondition {
      condition     = length(local.ingress_trust) <= 1
      error_message = "More than one backend says what is in front of the service with trusted_proxies. Use one ingress-* module for that; a media host returns none."
    }
    precondition {
      condition     = !strcontains(local.jobs_url, ".run.app") || (startswith(local.jobs_url, "https://${var.name}-") && endswith(local.jobs_url, ".${var.region}.run.app"))
      error_message = "MY_QUEUE_CLOUDTASKS_URL is ${local.jobs_url}, which is not the URL of the service ${var.name} in ${var.region}, so jobs would go nowhere. Give queue-gcp-cloud-tasks the same name and region."
    }
    precondition {
      condition     = !local.kek_missing
      error_message = "No KEK: with key_manager = ${local.key_manager} the server needs ${local.key_manager == "local" ? "kek" : "kms_key"}. Add a kek-* module to backends."
    }
  }
}

# Each binding goes on its resource unless the backend says project. The keys are strings known at plan time, which the
# backend modules build from their inputs.
resource "google_project_iam_member" "backend" {
  for_each = { for r in local.permissions : r.permission => r if r.scope == "project" }

  project = var.project_id
  role    = each.value.permission
  member  = google_service_account.server.member
}

resource "google_storage_bucket_iam_member" "backend" {
  for_each = { for r in local.permissions : "${r.resource}/${r.permission}" => r if r.scope == "bucket" }

  bucket = each.value.resource
  role   = each.value.permission
  member = google_service_account.server.member
}

resource "google_pubsub_topic_iam_member" "backend" {
  for_each = { for r in local.permissions : "${r.resource}/${r.permission}" => r if r.scope == "topic" }

  # projects/<project>/topics/<topic>
  project = split("/", each.value.resource)[1]
  topic   = split("/", each.value.resource)[3]
  role    = each.value.permission
  member  = google_service_account.server.member
}

resource "google_cloud_tasks_queue_iam_member" "backend" {
  for_each = { for r in local.permissions : "${r.resource}/${r.permission}" => r if r.scope == "queue" }

  # projects/<project>/locations/<location>/queues/<queue>
  project  = split("/", each.value.resource)[1]
  location = split("/", each.value.resource)[3]
  name     = split("/", each.value.resource)[5]
  role     = each.value.permission
  member   = google_service_account.server.member
}

resource "google_service_account_iam_member" "backend" {
  for_each = { for r in local.permissions : "${r.resource}/${r.permission}" => r if r.scope == "service_account" }

  service_account_id = each.value.resource
  role               = each.value.permission
  member             = google_service_account.server.member
}

resource "google_kms_crypto_key_iam_member" "backend" {
  for_each = { for r in local.permissions : "${r.resource}/${r.permission}" => r if r.scope == "crypto_key" }

  crypto_key_id = each.value.resource
  role          = each.value.permission
  member        = google_service_account.server.member
}

resource "google_secret_manager_secret_iam_member" "server" {
  for_each = local.secret_ids

  project   = var.project_id
  secret_id = each.value
  role      = "roles/secretmanager.secretAccessor"
  member    = google_service_account.server.member
}

resource "google_cloud_run_v2_service" "server" {
  project  = var.project_id
  location = var.region
  name     = var.name
  ingress  = local.cloud_run_ingress[var.service_ingress]
  # Cloud Tasks and Cloud Scheduler call this URL, and count as internal only on it.
  default_uri_disabled = false
  deletion_protection  = var.deletion_protection

  template {
    service_account                  = google_service_account.server.email
    timeout                          = "${var.request_timeout_seconds}s"
    max_instance_request_concurrency = var.concurrency

    scaling {
      min_instance_count = 0
      max_instance_count = var.max_instances
    }

    dynamic "vpc_access" {
      for_each = var.vpc_access == null ? [] : [var.vpc_access]
      content {
        egress = "PRIVATE_RANGES_ONLY"
        network_interfaces {
          network    = vpc_access.value.network
          subnetwork = vpc_access.value.subnetwork
        }
      }
    }

    dynamic "volumes" {
      for_each = local.file_mounts
      content {
        name = volumes.value.volume
        secret {
          secret = volumes.value.secret
          items {
            version = "latest"
            path    = "value"
          }
        }
      }
    }

    containers {
      image = var.image

      ports {
        container_port = 8080
      }

      resources {
        limits = {
          cpu    = var.cpu
          memory = "${var.memory_mib}Mi"
        }
        # Request-based billing: CPU only while a request runs. With queue = cloudtasks no process needs it otherwise,
        # since nothing polls.
        cpu_idle          = true
        startup_cpu_boost = true
      }

      env {
        name  = "GOMEMLIMIT"
        value = "${floor(var.memory_mib * 0.9)}MiB"
      }

      dynamic "env" {
        for_each = local.env
        content {
          name  = env.key
          value = env.value
        }
      }

      dynamic "env" {
        for_each = local.secret_env
        content {
          name = env.key
          value_source {
            secret_key_ref {
              secret  = env.value
              version = "latest"
            }
          }
        }
      }

      dynamic "volume_mounts" {
        for_each = local.file_mounts
        content {
          name       = volume_mounts.value.volume
          mount_path = volume_mounts.value.path
        }
      }

      # A revision takes traffic only when the schema is in place and every data upgrade step of its release is
      # recorded, which also holds back a revision rolled out before the migration.
      startup_probe {
        http_get {
          path = "/readyz"
          port = 8080
        }
        timeout_seconds   = 5
        period_seconds    = 10
        failure_threshold = 30
      }

      liveness_probe {
        http_get {
          path = "/healthz"
          port = 8080
        }
        timeout_seconds   = 5
        period_seconds    = 30
        failure_threshold = 3
      }
    }
  }

  traffic {
    type    = "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST"
    percent = 100
  }

  # The upgrade script changes the image, after the migration. An apply with another image would skip that order, so
  # apply keeps what runs. gcloud also writes the other fields listed here.
  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].containers[0].image,
      template[0].revision,
      template[0].labels,
      template[0].annotations,
    ]
  }

  depends_on = [
    google_project_iam_member.backend,
    google_storage_bucket_iam_member.backend,
    google_pubsub_topic_iam_member.backend,
    google_cloud_tasks_queue_iam_member.backend,
    google_service_account_iam_member.backend,
    google_kms_crypto_key_iam_member.backend,
    google_secret_manager_secret_iam_member.server,
  ]
}

# The one-off jobs share everything with the service except the command, so one block makes both: the migration job,
# which always exists, and the console, which exists while console_enabled is on. An execution overrides the arguments;
# the ones here change nothing if someone starts the job by hand.
resource "google_cloud_run_v2_job" "one_off" {
  for_each = local.jobs

  project             = var.project_id
  location            = var.region
  name                = "${var.name}-${each.key}"
  deletion_protection = var.deletion_protection

  template {
    task_count  = 1
    parallelism = 1

    template {
      service_account = google_service_account.server.email
      timeout         = "${var.job_timeout_seconds}s"
      # A failed step is for the operator to look at, and only one step may run at a time.
      max_retries = 0

      dynamic "vpc_access" {
        for_each = var.vpc_access == null ? [] : [var.vpc_access]
        content {
          egress = "PRIVATE_RANGES_ONLY"
          network_interfaces {
            network    = vpc_access.value.network
            subnetwork = vpc_access.value.subnetwork
          }
        }
      }

      dynamic "volumes" {
        for_each = local.file_mounts
        content {
          name = volumes.value.volume
          secret {
            secret = volumes.value.secret
            items {
              version = "latest"
              path    = "value"
            }
          }
        }
      }

      containers {
        image = var.image
        args  = each.value

        resources {
          limits = {
            cpu    = "1"
            memory = "${var.job_memory_mib}Mi"
          }
        }

        env {
          name  = "GOMEMLIMIT"
          value = "${floor(var.job_memory_mib * 0.9)}MiB"
        }

        dynamic "env" {
          for_each = local.env
          content {
            name  = env.key
            value = env.value
          }
        }

        dynamic "env" {
          for_each = local.secret_env
          content {
            name = env.key
            value_source {
              secret_key_ref {
                secret  = env.value
                version = "latest"
              }
            }
          }
        }

        dynamic "volume_mounts" {
          for_each = local.file_mounts
          content {
            name       = volume_mounts.value.volume
            mount_path = volume_mounts.value.path
          }
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [
      client,
      client_version,
      template[0].template[0].containers[0].image,
      template[0].labels,
      template[0].annotations,
    ]
  }

  depends_on = [
    google_project_iam_member.backend,
    google_storage_bucket_iam_member.backend,
    google_pubsub_topic_iam_member.backend,
    google_cloud_tasks_queue_iam_member.backend,
    google_service_account_iam_member.backend,
    google_kms_crypto_key_iam_member.backend,
    google_secret_manager_secret_iam_member.server,
  ]
}

# The server checks the OIDC token of /_internal/* itself, and its other paths are public: federation needs them.
resource "google_cloud_run_v2_service_iam_member" "public" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.server.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

resource "google_cloud_run_v2_service_iam_member" "invoker" {
  for_each = local.invokers

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.server.name
  role     = "roles/run.invoker"
  member   = "serviceAccount:${each.value}"
}
