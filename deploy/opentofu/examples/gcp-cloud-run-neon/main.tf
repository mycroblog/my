# SPDX-License-Identifier: AGPL-3.0-only

# One deployment on Google Cloud: Cloud Run, Cloud Tasks, Cloud Storage, and Pub/Sub behind a global external Application Load
# Balancer, with a database created by hand on Neon and email through an SMTP service. Each section of mycroblog-gcp picks
# another backend by its kind: deploy/opentofu/README.md lists them.

terraform {
  required_version = ">= 1.11"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 7.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

module "mycroblog" {
  source = "git::https://github.com/mycroblog/my.git//deploy/opentofu/mycroblog-gcp?ref=v0.1.0&depth=1"

  host  = var.host
  image = var.image
  media = { bucket_name = var.bucket_name }
  mail = {
    from = var.mail_from
    smtp = { host = var.mail_smtp_host, username = var.mail_smtp_username }
  }

  console_enabled = var.console_enabled

  # The recommended setup is the default of every section. To try a deployment cheaply, without the load balancer's monthly
  # fee, and with the run.app URL open:
  #
  #   ingress = { kind = "domain-mapping" }
  #
  # To give the delivery of federation its own service and queue, which scale apart from the default service:
  #
  #   workers = { delivery = { job_types = ["federation.fanout", "federation.deliver"], max_instances = 20, concurrency = 16 } }
}
