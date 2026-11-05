# SPDX-License-Identifier: AGPL-3.0-only

variable "image" {
  description = "Image at creation. The released image from ghcr.io runs as it is, given a route to the internet; an image in Amazon ECR needs none beyond ECR's endpoints. After creation the upgrade script changes it, and apply follows."
  type        = string
  nullable    = false
}

variable "architecture" {
  description = "Architecture to run: x86_64 or arm64. The released image has both."
  type        = string
  default     = "x86_64"
  nullable    = false

  validation {
    condition     = contains(["x86_64", "arm64"], var.architecture)
    error_message = "architecture must be x86_64 or arm64."
  }
}

variable "host" {
  description = "Public domain of the server. It cannot change once the server has started."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the cluster, the service, and its task definition family, and the start of every other name. upgrade.sh takes the same value as --name."
  type        = string
  default     = "my"
  nullable    = false

  validation {
    condition     = can(regex("^[a-zA-Z0-9][a-zA-Z0-9-]{0,27}$", var.name))
    error_message = "name must be 1 to 28 letters, digits, and hyphens, starting with a letter or a digit."
  }
}

variable "profile" {
  description = "profile of the server."
  type        = string
  default     = "production"
  nullable    = false
}

variable "network" {
  description = "The network of the tasks, from network-aws-vpc or an object of the same shape for a VPC of yours. A task takes its public subnets with assign_public_ip, and its private subnets otherwise."
  type = object({
    platform            = string
    vpc_id              = optional(string)
    vpc_cidr            = optional(string)
    public_subnet_ids   = optional(list(string), [])
    public_subnet_cidrs = optional(list(string), [])
    private_subnet_ids  = optional(list(string), [])
    nat                 = optional(bool, false)
  })
  nullable = false

  validation {
    condition     = var.network.platform == "aws"
    error_message = "network is of another platform: on AWS use network-aws-vpc, or an object of the shape in the README of deploy/opentofu."
  }

  validation {
    condition     = var.network.vpc_id != null
    error_message = "network.vpc_id is required."
  }

  validation {
    condition     = length(var.assign_public_ip ? var.network.public_subnet_ids : var.network.private_subnet_ids) > 0
    error_message = "The tasks take network.public_subnet_ids with assign_public_ip = true and network.private_subnet_ids otherwise, and the one they would take is empty."
  }
}

variable "assign_public_ip" {
  description = "Gives each task a public IPv4 address, which AWS bills by the hour, in a public subnet of the network. False takes a private subnet, with the network's NAT gateway as its way out, which AWS bills too. No default: it is the operator's choice and the operator's cost. Inbound traffic is closed either way."
  type        = bool
  nullable    = false
}

variable "backends" {
  description = "What each backend module returns. A setting named twice fails the plan."
  type = list(object({
    # gcp, aws, or neutral: what this module refuses is below.
    platform     = string
    env          = optional(map(string), {})
    secret_env   = optional(map(string), {})
    secret_files = optional(map(string), {})
    # What the migration and the console take in place of a setting of secret_env, which the service keeps.
    direct_secret_env = optional(map(string), {})
    # Security groups the tasks join to reach a backend in the VPC.
    security_group_ids = optional(list(string), [])
    # The VPC a backend has private addresses in, which must be the VPC of the tasks.
    private_network = optional(string)
    permissions = optional(list(object({
      permission = string
      resource   = optional(string)
      conditions = optional(list(object({
        test     = string
        variable = string
        values   = list(string)
      })), [])
    })), [])
    # What an ingress needs.
    service_ingress = optional(string)
    trusted_proxies = optional(list(string))
    load_balancer = optional(object({
      target_group_arn  = string
      security_group_id = string
    }))
    # What the backend adds to the one setting MY_BILLING_SOURCES.
    billing_sources = optional(list(string), [])
    # Accepted so that it cannot vanish, and refused below: nothing calls or invokes this service that way.
    invokers = optional(list(string), [])
    # Accepted so that they cannot vanish, and refused below: they belong to Google Cloud.
    cloud_sql_instances = optional(list(string), [])
    event_sources = optional(map(object({
      arn                        = string
      visibility_timeout_seconds = number
    })), {})
  }))
  default  = []
  nullable = false

  validation {
    condition     = alltrue([for b in var.backends : contains(["gcp", "aws", "neutral"], b.platform)])
    error_message = "A backend's platform is gcp, aws, or neutral."
  }

  # Modules of one cloud work only with that cloud's app: read "Platforms" in the README of deploy/opentofu.
  validation {
    condition     = alltrue([for b in var.backends : b.platform != "gcp"])
    error_message = "A module of Google Cloud works only with app-gcp-cloud-run, and one is in backends here, naming: ${join(", ", flatten([for b in var.backends : concat(keys(b.env), keys(b.secret_env), keys(b.secret_files)) if b.platform == "gcp"]))}. Use the module of this platform or a neutral one: blob-aws-s3, blob-s3, or blob-cloudflare-r2 in place of blob-gcp-gcs; db-aws-rds, db-external, or db-neon in place of db-gcp-cloud-sql; queue-db in place of queue-gcp-cloud-tasks; stream-aws-sns, stream-valkey, or stream-postgres in place of stream-gcp-pubsub; kv-aws-elasticache or kv-external in place of kv-gcp-memorystore; secrets-aws-secrets-manager in place of secrets-gcp-secret-manager; kek-aws-secrets-manager or kek-aws-kms in place of kek-gcp-*; ingress-aws-alb or ingress-cloudflare-app in place of ingress-gcp-*; monitoring-aws in place of monitoring-gcp. billing-gcp has no place on AWS."
  }

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [
        for r in b.permissions : r.resource != null && can(regex("^[a-z0-9-]+:[A-Za-z0-9*]+$", r.permission))
      ] if b.platform != "gcp"
    ]))
    error_message = "Each permission is one IAM action, such as sqs:SendMessage, with a resource, which is an ARN, an ARN pattern, or *."
  }

  validation {
    condition     = alltrue([for b in var.backends : b.service_ingress == null || b.service_ingress == "load-balancer"])
    error_message = "A task has no public host of its own, and its address changes with every task, so traffic reaches this service only through a load balancer it is attached to, such as ingress-aws-alb. An ingress that connects from the internet to a host (all) or one for Cloud Run does not work here. Read the README."
  }

  validation {
    condition     = alltrue([for b in var.backends : length(b.invokers) == 0])
    error_message = "invokers belongs to a platform that calls the app. A process here stays up and takes its jobs from the database: use queue-db."
  }

  validation {
    condition     = alltrue([for b in var.backends : length(b.event_sources) == 0])
    error_message = "event_sources belongs to a platform that polls a queue to invoke the app, and nothing does here: queue = sqs needs Lambda. Use queue-db."
  }

  validation {
    condition     = alltrue([for b in var.backends : length(b.cloud_sql_instances) == 0])
    error_message = "cloud_sql_instances belongs to Cloud Run. A backend in a VPC here returns security_group_ids and private_network."
  }

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [for name in keys(b.secret_files) : endswith(name, "_FILE")]
    ]))
    error_message = "The names of secret_files end in _FILE."
  }
}

variable "env" {
  description = "Other settings by variable name, for everything no backend covers."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_env" {
  description = "Other secret settings: variable name to the ARN of a secret in Secrets Manager. The execution role gets access to each."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_files" {
  description = "The same as secret_env under the name ending in _FILE, since Fargate mounts no secret: variable name to the ARN of a secret."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = alltrue([for name in keys(var.secret_files) : endswith(name, "_FILE")])
    error_message = "The names of secret_files end in _FILE."
  }
}

variable "cpu" {
  description = "CPU units of a service task, from the sizes Fargate offers with memory_mib. AWS bills them for as long as the task runs, idle or not."
  type        = number
  default     = 256
  nullable    = false
}

variable "memory_mib" {
  description = "Memory of a service task in MiB. GOMEMLIMIT is set to 90% of it."
  type        = number
  default     = 1024
  nullable    = false
}

variable "desired_count" {
  description = "Tasks of the service. Above 1 needs a stream other than hub, which the plan checks. An upgrade runs one more for a while."
  type        = number
  default     = 1
  nullable    = false

  validation {
    condition     = var.desired_count >= 1
    error_message = "desired_count must be at least 1. To stop the server, use the maintenance mode or destroy the service."
  }
}

variable "job_cpu" {
  description = "CPU units of the migration and console tasks, billed only while one runs."
  type        = number
  default     = 512
  nullable    = false
}

variable "job_memory_mib" {
  description = "Memory of the migration and console tasks in MiB."
  type        = number
  default     = 1024
  nullable    = false
}

variable "health_check_grace_seconds" {
  description = "How long after a task starts the service ignores the load balancer's health checks. Used only with a load balancer."
  type        = number
  default     = 120
  nullable    = false
}

variable "console_enabled" {
  description = "Creates the console's task definition. Turn it on for as long as you need it."
  type        = bool
  default     = false
  nullable    = false
}

variable "log_retention_days" {
  description = "How long CloudWatch Logs keeps the logs of the tasks."
  type        = number
  default     = 30
  nullable    = false
}
