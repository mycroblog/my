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
  media = { bucket_name = var.bucket_name }
  mail = {
    from = var.mail_from
    smtp = { host = var.mail_smtp_host, username = var.mail_smtp_username }
  }
  # Who the alerts notify. null creates no uptime check and no alerts.
  monitoring = { notification_emails = var.alert_emails }

  console_enabled = var.console_enabled

  # The image is the release in ref, so raising ref upgrades: the apply runs the migration, then moves the services. To pin
  # the image by its digest, from the release notes:
  #
  #   image = { digest = "sha256:..." }
  #
  # The recommended setup is the default of every section. To try a deployment cheaply, without the load balancer's monthly
  # fee, and with the run.app URL open:
  #
  #   ingress = { kind = "domain-mapping" }
  #
  # To give the delivery of federation its own service and queue, which scale apart from the default service:
  #
  #   workers = { delivery = { job_types = ["federation.fanout", "federation.deliver"], max_instances = 20, concurrency = 16 } }
}
