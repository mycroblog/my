# SPDX-License-Identifier: AGPL-3.0-only

variable "project_id" {
  description = "Project of the service, the jobs, and the service account."
  type        = string
  nullable    = false
}

variable "region" {
  description = "Region of the service and the jobs."
  type        = string
  nullable    = false
}

variable "name" {
  description = "Name of the service, and start of the names of the jobs and the service account."
  type        = string
  default     = "my"
  nullable    = false
}

variable "image" {
  description = "Server image at creation, as ghcr.io/mycroblog/my:<version>, optionally with @sha256:<digest>. After that only the upgrade script changes the image: apply keeps what runs."
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("(:[^/:@]+|@sha256:[0-9a-f]{64})$", var.image)) && !can(regex(":latest(@|$)", var.image))
    error_message = "image must name a version or a digest. There is no latest tag."
  }
}

variable "host" {
  description = "Domain of the server (host). It cannot change once the server has started."
  type        = string
  nullable    = false
}

variable "role" {
  description = "role of the service. Cloud Tasks and Cloud Scheduler call it, so it must run jobs: all or worker."
  type        = string
  default     = "all"
  nullable    = false

  validation {
    condition     = contains(["all", "worker"], var.role)
    error_message = "role must be all or worker: the endpoints that Cloud Tasks and Cloud Scheduler call are not served by web."
  }
}

variable "profile" {
  description = "profile of the server."
  type        = string
  default     = "production"
  nullable    = false
}

variable "service_ingress" {
  description = "Who may reach the service: all, the internet; internal-and-load-balancer, only the load balancer and internal traffic; internal, only internal traffic. No default: the choice is what decides whether anyone can go around an ingress. Cloud Tasks and Cloud Scheduler of the same project reach it under any of them."
  type        = string
  nullable    = false

  validation {
    condition     = contains(["all", "internal", "internal-and-load-balancer"], var.service_ingress)
    error_message = "service_ingress must be all, internal, or internal-and-load-balancer."
  }
}

variable "network" {
  description = "The network that the service and the jobs attach to through Direct VPC egress, from network-gcp-vpc or an object of the same shape for a VPC of yours. Null, the default, attaches nothing, which is all that a deployment on public endpoints needs. A backend with private addresses, such as kv-gcp-memorystore, fails the plan without the same network."
  type = object({
    platform                      = string
    network                       = optional(string)
    subnetwork                    = optional(string)
    private_services_range        = optional(string)
    memorystore_connection_policy = optional(bool, false)
    nat                           = optional(bool, false)
  })
  default = null

  validation {
    condition     = var.network == null ? true : var.network.platform == "gcp"
    error_message = "network is of another platform: on Google Cloud use network-gcp-vpc, or an object of the shape in the README of deploy/opentofu."
  }

  validation {
    condition     = var.network == null ? true : can(regex("^projects/[^/]+/global/networks/[^/]+$", var.network.network))
    error_message = "network.network must be projects/<project>/global/networks/<name>."
  }

  validation {
    condition     = var.network == null ? true : can(regex("^projects/[^/]+/regions/${var.region}/subnetworks/[^/]+$", var.network.subnetwork))
    error_message = "network.subnetwork must be projects/<project>/regions/<region>/subnetworks/<name>, in the region of the service: Direct VPC egress needs that."
  }
}

variable "vpc_egress" {
  description = "What goes through the network: PRIVATE_RANGES_ONLY, which keeps the service's own path to the internet, or ALL_TRAFFIC, which sends everything through the VPC and so needs Cloud NAT of the network for the internet that federation reaches."
  type        = string
  default     = "PRIVATE_RANGES_ONLY"
  nullable    = false

  validation {
    condition     = contains(["PRIVATE_RANGES_ONLY", "ALL_TRAFFIC"], var.vpc_egress)
    error_message = "vpc_egress must be PRIVATE_RANGES_ONLY or ALL_TRAFFIC."
  }

  validation {
    condition     = var.vpc_egress != "ALL_TRAFFIC" || try(var.network.nat, false)
    error_message = "With vpc_egress = ALL_TRAFFIC the internet is reached only through the VPC, so with no Cloud NAT federation stops. Set nat of the network, or leave vpc_egress at PRIVATE_RANGES_ONLY, which needs none."
  }
}

variable "backends" {
  description = "What each backend module returns. A setting named twice fails the plan."
  type = list(object({
    # gcp, aws, or neutral: what this module refuses is below.
    platform     = string
    env          = optional(map(string), {})
    secret_env   = optional(map(string), {})
    secret_files = optional(map(string), {})
    # What the one-off jobs take in place of a setting of secret_env, which the service keeps.
    direct_secret_env = optional(map(string), {})
    # Cloud SQL instances to reach through the connector's socket, as <project>:<region>:<instance>.
    cloud_sql_instances = optional(list(string), [])
    permissions = optional(list(object({
      permission = string
      scope      = optional(string)
      resource   = optional(string)
      # Accepted so that it cannot vanish, and refused below: Google Cloud's roles take no such conditions here.
      conditions = optional(list(object({
        test     = string
        variable = string
        values   = list(string)
      })), [])
    })), [])
    invokers = optional(list(string), [])
    # What an ingress needs of the service, which must be var.service_ingress.
    service_ingress = optional(string)
    # What an ingress puts in front of the service. The app adds Cloud Run's own front end.
    trusted_proxies = optional(list(string))
    # What the backend adds to the one setting MY_BILLING_SOURCES.
    billing_sources = optional(list(string), [])
    # The network a backend has private addresses on, which must be the network of the app.
    private_network = optional(string)
    # Accepted so that they cannot vanish, and refused below: they belong to AWS.
    security_group_ids = optional(list(string), [])
    event_sources = optional(map(object({
      arn                        = string
      visibility_timeout_seconds = number
    })), {})
    load_balancer = optional(object({
      target_group_arn  = string
      security_group_id = string
    }))
  }))
  default  = []
  nullable = false

  validation {
    condition     = alltrue([for b in var.backends : contains(["gcp", "aws", "neutral"], b.platform)])
    error_message = "A backend's platform is gcp, aws, or neutral."
  }

  # Modules of one cloud work only with that cloud's app: read "Platforms" in the README of deploy/opentofu. The one
  # exception is stated here and nowhere else: billing-aws with web_identity, since reading another cloud's costs is only an
  # outside API call. Its role holds the permission, so it asks this app for nothing, and without web_identity it would.
  validation {
    condition = alltrue([
      for b in var.backends : b.platform != "aws" || (join(",", b.billing_sources) == "aws" && length(b.permissions) == 0)
    ])
    error_message = "A module of AWS works only with app-aws-ecs or app-aws-lambda, and one is in backends here, naming: ${join(", ", flatten([for b in var.backends : concat(keys(b.env), keys(b.secret_env), keys(b.secret_files)) if b.platform == "aws"]))}. Use the module of this platform or a neutral one: blob-gcp-gcs, blob-s3 with keys, or blob-cloudflare-r2 in place of blob-aws-s3; db-gcp-cloud-sql, db-external, or db-neon in place of db-aws-rds; queue-gcp-cloud-tasks in place of queue-aws-sqs; stream-gcp-pubsub, stream-valkey, or stream-postgres in place of stream-aws-sns; kv-gcp-memorystore or kv-external in place of kv-aws-elasticache; mail-smtp in place of mail-aws-ses; secrets-gcp-secret-manager in place of secrets-aws-secrets-manager; kek-gcp-secret-manager or kek-gcp-kms in place of kek-aws-*; ingress-gcp-domain-mapping, ingress-gcp-load-balancer, or ingress-cloudflare-app in place of ingress-aws-*; monitoring-gcp in place of monitoring-aws. billing-aws is the one exception, with web_identity = true."
  }

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [
        for r in b.permissions : (
          contains(["project", "bucket", "topic", "queue", "service_account", "crypto_key", "dataset", "subscription"], r.scope == null ? "" : r.scope) &&
          (r.scope == "project" || r.resource != null)
        )
      ] if b.platform != "aws"
    ]))
    error_message = "Each permission has a scope of project (with another project as its resource, or none for this one), or bucket, topic, queue, service_account, crypto_key, dataset, or subscription (with its resource)."
  }

  validation {
    condition     = alltrue(flatten([for b in var.backends : [for r in b.permissions : length(r.conditions) == 0]]))
    error_message = "A permission on Google Cloud has no conditions: a binding here is a role on a resource."
  }

  validation {
    condition     = alltrue([for b in var.backends : length(b.security_group_ids) == 0 && length(b.event_sources) == 0 && b.load_balancer == null])
    error_message = "security_group_ids, event_sources, and load_balancer belong to AWS. A backend on private addresses here returns private_network."
  }

  validation {
    condition = alltrue([
      for b in var.backends : b.service_ingress == null || contains(["all", "internal", "internal-and-load-balancer"], b.service_ingress)
    ])
    error_message = "service_ingress must be all, internal, or internal-and-load-balancer."
  }

  validation {
    condition = alltrue(flatten([
      for b in var.backends : [for name in keys(b.secret_files) : endswith(name, "_FILE")]
    ]))
    error_message = "The names of secret_files end in _FILE: the variable holds the path of the mounted file."
  }
}

variable "env" {
  description = "Other settings by variable name, for everything no backend covers."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_env" {
  description = "Other secret settings: variable name to Secret Manager secret ID. The service account gets access to each."
  type        = map(string)
  default     = {}
  nullable    = false
}

variable "secret_files" {
  description = "Other secret settings to mount as files: variable name, ending in _FILE, to Secret Manager secret ID."
  type        = map(string)
  default     = {}
  nullable    = false

  validation {
    condition     = alltrue([for name in keys(var.secret_files) : endswith(name, "_FILE")])
    error_message = "The names of secret_files end in _FILE: the variable holds the path of the mounted file."
  }
}

variable "cpu" {
  description = "CPUs of an instance. Below 1, Cloud Run takes one request at a time."
  type        = string
  default     = "1"
  nullable    = false
}

variable "memory_mib" {
  description = "Memory of an instance in MiB. GOMEMLIMIT is set to 90% of it."
  type        = number
  default     = 512
  nullable    = false
}

variable "max_instances" {
  description = "Most instances at once. With pool_max_conns in the database URL, it bounds the connections to the database."
  type        = number
  default     = 3
  nullable    = false
}

variable "min_instances" {
  description = "Fewest instances that Cloud Run keeps running. queue = db needs at least 1, together with cpu_always_allocated. Each one kept is billed while idle."
  type        = number
  default     = 0
  nullable    = false

  validation {
    condition     = var.min_instances >= 0 && floor(var.min_instances) == var.min_instances && var.min_instances <= var.max_instances
    error_message = "min_instances must be a whole number from 0 to max_instances."
  }
}

variable "cpu_always_allocated" {
  description = "Allocates CPU outside requests as well, which is instance-based billing: an instance is billed at the full rate for as long as it runs. Off, CPU is allocated only during requests. queue = db needs it, together with min_instances of at least 1."
  type        = bool
  default     = false
  nullable    = false
}

variable "concurrency" {
  description = "Requests per instance. An open stream of real-time updates counts as one."
  type        = number
  default     = 80
  nullable    = false
}

variable "request_timeout_seconds" {
  description = "Request timeout of the service. It must exceed queue_cloudtasks_dispatch_deadline, 10 minutes by default."
  type        = number
  default     = 900
  nullable    = false
}

variable "job_memory_mib" {
  description = "Memory of the migration and console jobs in MiB."
  type        = number
  default     = 1024
  nullable    = false
}

variable "job_timeout_seconds" {
  description = "How long one job execution may run."
  type        = number
  default     = 3600
  nullable    = false
}

variable "console_enabled" {
  description = "Creates the console job. Turn it on for as long as you need it."
  type        = bool
  default     = false
  nullable    = false
}

variable "deletion_protection" {
  description = "Refuses to delete the service and the jobs."
  type        = bool
  default     = true
  nullable    = false
}
