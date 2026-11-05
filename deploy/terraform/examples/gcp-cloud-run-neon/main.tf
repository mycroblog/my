# SPDX-License-Identifier: AGPL-3.0-only

# One deployment: Cloud Run, Cloud Tasks, Cloud Storage, and Pub/Sub, with a database created by hand on Neon and email
# through an SMTP service. To change a backend, replace its module block: the app module takes whatever the block returns, by the contract in
# deploy/terraform/README.md.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# The first apply generates the VAPID key. Add the others with gcloud secrets versions add before the app: they come from
# elsewhere, and the guide has the order.
module "secrets" {
  source = "../../modules/secrets-gcp-secret-manager"

  project_id         = var.project_id
  secrets            = { MY_VAPID_PRIVATE_KEY = var.vapid_secret_id }
  generate_p256_keys = ["MY_VAPID_PRIVATE_KEY"]
  referenced = {
    database_url  = var.database_url_secret_id
    smtp_password = var.mail_smtp_password_secret_id
  }
}

# To keep the KEK in Cloud KMS instead, replace this block. On an instance with data this is a move: read the guide first.
#
#   module "kek" {
#     source = "../../modules/kek-gcp-kms"
#
#     project_id = var.project_id
#     location   = var.region
#   }
# The first apply generates the KEK. Copy it right away: the guide says how.
module "kek" {
  source = "../../modules/kek-gcp-secret-manager"

  project_id = var.project_id
  secret_id  = var.kek_secret_id
}

module "db" {
  source = "../../modules/db-external"

  url_secret_id = module.secrets.ids["database_url"]
}

module "queue" {
  source = "../../modules/queue-gcp-cloud-tasks"

  project_id    = var.project_id
  region        = var.region
  host          = var.host
  tick_schedule = var.tick_schedule
}

module "blob" {
  source = "../../modules/blob-gcp-gcs"

  project_id  = var.project_id
  bucket_name = var.bucket_name
  location    = var.bucket_location
}

module "stream" {
  source = "../../modules/stream-gcp-pubsub"

  project_id = var.project_id
}

module "mail" {
  source = "../../modules/mail-smtp"

  from               = var.mail_from
  host               = var.mail_smtp_host
  port               = var.mail_smtp_port
  username           = var.mail_smtp_username
  password_secret_id = module.secrets.ids["smtp_password"]
}

module "app" {
  source = "../../modules/app-gcp-cloud-run"

  project_id      = var.project_id
  region          = var.region
  image           = var.image
  host            = var.host
  console_enabled = var.console_enabled
  domain_mapping  = var.domain_mapping

  backends = [module.secrets, module.kek, module.db, module.queue, module.blob, module.stream, module.mail]

  env = merge(
    # Cloud Run refuses an HTTP/1 request above 32 MiB, and the server speaks HTTP/1.1. var.env can set another value.
    { MY_MEDIA_MAX_AV_BYTES = "31457280" },
    var.env,
  )
  secret_env = var.secret_env
}
