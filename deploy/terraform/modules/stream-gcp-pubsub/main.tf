# SPDX-License-Identifier: AGPL-3.0-only

terraform {
  required_version = ">= 1.9"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 7.0, < 8.0"
    }
  }
}

variable "project_id" {
  description = "Project of the topic and of the subscriptions the server creates."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the topic and start of the subscription IDs, used by this instance alone."
  type        = string
  default     = "my-stream"
  nullable    = false

  validation {
    condition     = can(regex("^[A-Za-z][A-Za-z0-9_.~+%-]*$", var.name)) && !startswith(var.name, "goog")
    error_message = "name must start with a letter, hold letters, digits, and -_.~+%, and not start with goog."
  }
}

locals {
  # A custom role's ID allows letters, digits, underscores, and periods only.
  role_id = "${replace(var.name, "/[^A-Za-z0-9_.]/", "_")}_subscriptions"
  # Built from the inputs so that they are known at plan time: the app module keys its IAM bindings on them. The
  # outputs depend on the resources for the order.
  topic = "projects/${var.project_id}/topics/${var.name}"
  role  = "projects/${var.project_id}/roles/${local.role_id}"
}

resource "google_pubsub_topic" "stream" {
  project = var.project_id
  name    = var.name
}

# The server creates one subscription per process and deletes it, so their names are not known in advance and these
# permissions go on the project. roles/pubsub.editor would grant far more.
resource "google_project_iam_custom_role" "subscriptions" {
  project     = var.project_id
  role_id     = local.role_id
  title       = "${var.name} stream subscriptions"
  description = "Lets the server create, read, and delete its own per-process subscriptions."
  permissions = [
    "pubsub.subscriptions.consume",
    "pubsub.subscriptions.create",
    "pubsub.subscriptions.delete",
    "pubsub.subscriptions.list",
  ]
}

output "env" {
  description = "The settings of the stream."
  value = {
    MY_STREAM                            = "pubsub"
    MY_STREAM_PUBSUB_TOPIC               = local.topic
    MY_STREAM_PUBSUB_SUBSCRIPTION_PREFIX = var.name
  }
  depends_on = [google_pubsub_topic.stream]
}

output "secret_env" {
  description = "No secrets: the server authenticates with its service account."
  value       = {}
}

output "roles" {
  description = "Publishing to the topic, attaching subscriptions to it, and managing the subscriptions of the project."
  value = [
    { role = "roles/pubsub.publisher", scope = "topic", resource = local.topic },
    # Holds pubsub.topics.attachSubscription.
    { role = "roles/pubsub.subscriber", scope = "topic", resource = local.topic },
    { role = local.role, scope = "project", resource = null },
  ]
  depends_on = [google_pubsub_topic.stream, google_project_iam_custom_role.subscriptions]
}
